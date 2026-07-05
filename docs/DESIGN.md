# Design Document — Distributed Job Scheduler

## 1. System Components

```
                         ┌──────────────────────┐
                         │   Next.js Dashboard   │
                         │  (App Router, SSR)    │
                         └──────────┬───────────┘
                                    │ Route Handlers
                                    │ (proxy, never direct)
                                    ▼
┌──────────────┐  HTTP   ┌──────────────────────┐
│  Clients /   │────────▶│     API Server        │
│  Webhooks    │         │  (cmd/api)            │
└──────────────┘         └──────────┬───────────┘
                                    │ pgxpool
                                    ▼
                         ┌──────────────────────┐
                    ┌───▶│     PostgreSQL        │◀───┐
                    │    │  (single source of    │    │
                    │    │   truth for all state)│    │
                    │    └──────────────────────┘    │
                    │ pgxpool                 pgxpool│
          ┌─────────┴──────────┐        ┌───────────┴──────┐
          │   Worker Pool      │        │     Reaper        │
          │  (cmd/worker)      │        │  (cmd/reaper)     │
          │  N instances,      │        │  single cron-loop │
          │  M goroutines each │        │  reclaims expired │
          └────────────────────┘        │  leases           │
                                        └──────────────────┘
```

**Interaction summary:**

| Component | Role | Talks to |
|-----------|------|----------|
| **API Server** | Accepts job submissions (enqueue), serves job/queue status, cancels jobs. Stateless HTTP process. | PostgreSQL |
| **Worker Pool** | Polls Postgres for claimable jobs, executes them, sends heartbeats. Horizontally scalable — run N replicas. | PostgreSQL |
| **Reaper** | Periodic loop (every ~10 s). Finds jobs with `lease_expires_at < NOW()` and transitions them back to `pending` (retry) or `dead` (exhausted). | PostgreSQL |
| **PostgreSQL** | The only shared state. All coordination happens through row-level locks and atomic SQL. No external broker. | — |
| **Dashboard** | Read-only operational view. Polls the API for queue depth, job status, failure rates. Never writes directly to Postgres. | API Server |

There is no message broker, no Redis, no ZooKeeper. Postgres is the queue, the lock manager, and the datastore.

---

## 2. Job Lifecycle State Machine

```
                          enqueue
                            │
                            ▼
                       ┌─────────┐
                       │ pending  │◀──────────────────────────┐
                       └────┬────┘                            │
                            │ run_at <= NOW()                 │
                            │ or run_at IS NULL               │
                            ▼                                 │
                       ┌─────────┐                            │
                       │scheduled│  (logical; same row,       │
                       │         │   pending + run_at in past) │
                       └────┬────┘                            │
                            │ atomic claim                    │
                            │ (UPDATE ... SKIP LOCKED)        │
                            ▼                                 │
                       ┌─────────┐    lease expired           │
                       │ running │────────────────┐           │
                       └────┬────┘                │           │
                            │                     ▼           │
              ┌─────────────┼──────────┐    ┌──────────┐      │
              │             │          │    │  reaper   │      │
              ▼             ▼          │    │ reclaims  │      │
        ┌───────────┐ ┌──────────┐     │    └────┬─────┘      │
        │ completed │ │  failed  │     │         │            │
        └───────────┘ └────┬─────┘     │         │            │
                           │           │         │            │
                           │ attempt   │◀────────┘            │
                           │ < max     │                      │
                           │ retries?  │                      │
                           │           │                      │
                      yes  │    no     │                      │
                      ┌────┘    └──────▼──────┐               │
                      │              ┌────────┴──┐            │
                      │              │dead_letter │            │
                      │              └───────────┘            │
                      │  compute next_run_at                  │
                      │  set status = 'pending'               │
                      └───────────────────────────────────────┘
```

**States:**

| State | Meaning | Mutable by |
|-------|---------|------------|
| `pending` | Waiting to be claimed. If `run_at` is set in the future, effectively "scheduled." | API (enqueue), Reaper (retry) |
| `running` | Claimed by a worker. `worker_id` and `lease_expires_at` are set. | Worker (claim) |
| `completed` | Terminal success. Immutable. | Worker |
| `failed` | Attempt failed. Transient — will be retried or moved to DLQ. | Worker, Reaper |
| `dead_letter` | Terminal failure. Exhausted all retries. Requires human intervention. | Reaper |

`scheduled` is not a distinct DB status — it is `pending` with `run_at > NOW()`. The claim query's `WHERE run_at <= NOW()` naturally skips future-scheduled jobs.

---

## 3. Atomic Claim Strategy

### The query

```sql
UPDATE jobs
SET    status           = 'running',
       worker_id        = $1,
       lease_expires_at = NOW() + $2::interval,
       started_at       = NOW()
WHERE  id IN (
    SELECT id FROM jobs
    WHERE  status = 'pending'
      AND  (run_at IS NULL OR run_at <= NOW())
    ORDER  BY priority DESC, created_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT  $3
)
RETURNING id, queue, payload, idempotency_key, attempt;
```

### Why this beats the alternatives

| Approach | Problem |
|----------|---------|
| **Naive SELECT then UPDATE** | Classic TOCTOU race. Two workers SELECT the same row, both UPDATE it. One silently overwrites the other's claim. You now have two workers executing the same job. Adding `WHERE status = 'pending'` to the UPDATE reduces but doesn't eliminate the window — under high concurrency the conflict rate grows linearly with worker count. |
| **SELECT ... FOR UPDATE (without SKIP LOCKED)** | Correct but serializing. Worker A locks row 1; worker B blocks waiting for that same row even though rows 2–1000 are available. Throughput collapses to single-threaded claim rate. Every worker contends on the same "next" row. |
| **Advisory locks (`pg_advisory_lock`)** | Requires a secondary mapping from job ID to lock ID. Lock lifecycle is tied to the session, not the transaction, which makes cleanup on crash subtle. You also lose the ability to batch-claim N jobs atomically — each lock is a separate round trip. Advisory locks solve a different problem (application-level mutexes), not row-level work distribution. |
| **Redis queue (BRPOPLPUSH / streams)** | Adds an external dependency that must be kept consistent with Postgres. Job metadata lives in Postgres; pointers live in Redis. Any network partition between them means either lost jobs or phantom claims. You now have two systems to back up, monitor, and reason about during failures. For throughput under ~10 k jobs/s, Postgres `SKIP LOCKED` is more than sufficient and eliminates the consistency boundary. |

**`FOR UPDATE SKIP LOCKED`** gives us:

1. **No contention** — locked rows are skipped, not waited on. N workers claim N disjoint batches concurrently.
2. **Atomicity** — claim + status transition + lease assignment happen in one statement, one round trip.
3. **Priority ordering** — the `ORDER BY` inside the subselect is respected; `SKIP LOCKED` only skips rows that are *currently* locked by another in-flight claim, not arbitrary rows.
4. **Single system** — Postgres is already the datastore; no new failure domains.

---

## 4. Lease / Heartbeat / Reaper — Crash Recovery

### The problem this solves

A worker is OOM-killed by the kernel (or segfaults, or its node is terminated) while
running job `J`. The process is gone — it cannot update the job's status. Without
intervention, `J` stays `running` forever.

### Mechanism

```
Worker claim                   Heartbeat ticks              OOM kill
    │                               │                          │
    ▼                               ▼                          ▼
┌────────┐  lease = +30s  ┌────────────────┐             ┌──────────┐
│ claim  │───────────────▶│ lease_expires  │──tick(10s)──▶│ process  │
│ job J  │                │ = T+30s        │  renew to   │ dies     │
└────────┘                │                │  T+40s,     │          │
                          │                │  T+50s, ... │          │
                          └────────────────┘             └──────────┘
                                                               │
                                              lease_expires_at │ is now
                                              in the past      │
                                                               ▼
                                                    ┌─────────────────┐
                                                    │ Reaper loop     │
                                                    │ (every ~10s)    │
                                                    │                 │
                                                    │ UPDATE jobs     │
                                                    │ SET status =    │
                                                    │   'failed'      │
                                                    │ WHERE status =  │
                                                    │   'running'     │
                                                    │ AND lease_      │
                                                    │  expires_at     │
                                                    │  < NOW()        │
                                                    └────────┬────────┘
                                                             │
                                                             ▼
                                                    retry or dead_letter
                                                    (based on attempt count)
```

**Parameters (tunable via env):**

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| Lease duration | 30 s | Long enough that a healthy worker never loses its lease. Short enough that a dead worker's job is recovered in under a minute. |
| Heartbeat interval | 10 s | At least 2 heartbeats fit inside one lease window — tolerates one missed heartbeat (GC pause, network blip) without false reaping. |
| Reaper interval | 10 s | Worst-case recovery time = lease duration + reaper interval = ~40 s. |

**Failure scenario walkthrough:**

1. Worker W1 claims job J, sets `lease_expires_at = NOW() + 30s`.
2. W1 sends heartbeats at T+10 s and T+20 s, each extending the lease by 30 s.
3. At T+22 s, W1 is OOM-killed. `lease_expires_at` is T+50 s (last renewal was at T+20 s).
4. No more heartbeats arrive. The lease expires at T+50 s.
5. Reaper runs at T+55 s, finds J with `status = 'running'` and `lease_expires_at < NOW()`.
6. Reaper sets J to `failed`, increments `attempt`, computes `next_run_at` per the retry strategy.
7. If `attempt < max_retries`, J goes back to `pending` with the computed `run_at`. Otherwise, J goes to `dead_letter`.
8. Another worker picks up J on its next claim cycle.

**Graceful shutdown** is the clean counterpart: a worker that receives SIGTERM stops claiming, waits for in-flight jobs to finish (with a deadline), then explicitly releases its leases by setting those jobs back to `pending`. The reaper never fires for graceful shutdowns.

---

## 5. Idempotency & Delivery Semantics

### Guarantee: at-least-once execution, effectively-once side effects

The scheduler guarantees **at-least-once delivery**: a job will be executed at least once before it reaches `completed` or `dead_letter`. It does *not* guarantee exactly-once execution — a worker can crash after performing a side effect but before committing the status update, causing the job to be re-executed after reaper recovery.

To achieve **effectively-once side effects**, every job carries an `idempotency_key` (a UUID or deterministic hash). The execution path is:

```
BEGIN TX
  ├── SELECT 1 FROM idempotency_keys WHERE key = $1 FOR UPDATE
  │     └── row exists? → COMMIT (no-op, already applied)
  │
  ├── ... perform side effect (send email, charge card, call API) ...
  │
  ├── INSERT INTO idempotency_keys (key, job_id) VALUES ($1, $2)
  ├── UPDATE jobs SET status = 'completed' WHERE id = $3
COMMIT
```

The `FOR UPDATE` on the idempotency row serializes concurrent attempts for the same key. The side effect and the idempotency record are committed atomically. If the transaction fails at any point, neither is persisted, and the retry is safe.

**Scope:** idempotency guards cover *our* side effects. If the side effect is an external API call (e.g., charging a payment provider), true idempotency depends on the external system also supporting idempotency keys. The scheduler passes the key through; the executor is responsible for forwarding it.

---

## 6. Retry Strategies

When a job fails (worker reports error, or reaper reclaims an expired lease), the system computes `next_run_at` based on the queue's configured retry strategy.

### Backoff formulas

| Strategy | `next_run_at` | Example (base = 5 s, attempt = 3) |
|----------|---------------|------------------------------------|
| **Fixed** | `NOW() + base` | `NOW() + 5s` |
| **Linear** | `NOW() + base × attempt` | `NOW() + 15s` |
| **Exponential** | `NOW() + base × 2^(attempt - 1)` | `NOW() + 20s` |

Exponential backoff is capped at a configurable `max_backoff` (default: 1 hour) to prevent jobs from disappearing for days.

### Retry flow

```
job fails (attempt N)
    │
    ▼
attempt < max_retries?
    │
   yes──▶ compute next_run_at per strategy
    │         │
    │         ▼
    │     UPDATE jobs SET
    │       status   = 'pending',
    │       run_at   = next_run_at,
    │       attempt  = attempt + 1
    │
   no───▶ UPDATE jobs SET
              status = 'dead_letter',
              died_at = NOW()
```

### Dead-letter queue

`dead_letter` is a terminal state. These jobs remain in the `jobs` table with `status = 'dead_letter'` for inspection. The dashboard surfaces them prominently. An operator can:

1. **Inspect** the payload and failure reason.
2. **Retry manually** via the API, which resets `attempt = 0`, `status = 'pending'`, clears `run_at`.
3. **Delete** if the job is no longer relevant.

There is no automatic recovery from `dead_letter`. This is intentional — if a job failed N times, it needs human eyes.

---

## 7. Trade-offs & What Was Deliberately Not Built

### 1. Postgres-as-queue vs. dedicated broker

**Chose:** Postgres with `SKIP LOCKED`.
**Gave up:** Raw throughput beyond ~10 k claims/s. A dedicated broker (Kafka, SQS, NATS JetStream) would push that ceiling to 100 k+.
**Why:** One fewer system to operate, back up, and monitor. For the target workload (order processing, notifications, async tasks), 10 k/s is more than enough. If it isn't, the claim query is the only component to replace — the domain layer is broker-agnostic by design.

### 2. Pull-based (polling) vs. push-based (LISTEN/NOTIFY)

**Chose:** Workers poll on a configurable interval (default 1 s).
**Gave up:** Sub-second latency for newly enqueued jobs. A `LISTEN/NOTIFY` approach would wake workers immediately.
**Why:** Polling is simpler to reason about under backpressure. NOTIFY doesn't carry the payload, so you still need a claim query after the wake-up. The added complexity (managing LISTEN connections, reconnect logic, thundering herd on NOTIFY) wasn't justified for a 1 s p99 enqueue-to-start latency.

### 3. Single reaper process vs. distributed reaper

**Chose:** Single reaper (or leader-elected via `pg_advisory_lock`).
**Gave up:** Reaper HA without an external leader-election mechanism.
**Why:** The reaper is stateless and crash-tolerant — if it dies, stale jobs simply wait an extra reaper interval. Running a second reaper instance with a `pg_try_advisory_lock` at the top of the loop is a one-line HA upgrade. Building a full Raft/consensus layer for the reaper would be over-engineering.

### 4. At-least-once vs. exactly-once execution

**Chose:** At-least-once execution with idempotency keys for effectively-once side effects.
**Gave up:** True exactly-once without any application-level cooperation.
**Why:** Exactly-once in a distributed system requires either two-phase commit (fragile, slow) or that every downstream system supports idempotency natively. At-least-once with idempotency keys pushes the dedup responsibility to the point where it can actually be enforced — inside the transaction boundary — without pretending the network is reliable.

### 5. No job dependencies / DAG execution

**Chose:** Each job is independent. No "run B after A completes" primitives.
**Gave up:** Workflow orchestration (Temporal, Airflow-style DAGs).
**Why:** DAG scheduling is a fundamentally different system with its own state machine, cycle detection, and fan-out/fan-in semantics. Bolting it onto a job scheduler creates a half-broken workflow engine. If DAGs are needed, use a dedicated orchestrator that enqueues leaf tasks into this scheduler.
