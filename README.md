# Distributed Job Scheduler

A production-grade distributed job scheduler built with **Go 1.22+**, **PostgreSQL**, and a **Next.js** dashboard. Postgres is the queue, the lock manager, and the datastore — no Redis, no ZooKeeper, no external broker.

## Architecture

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

| Component | Role |
|-----------|------|
| **API Server** | Accepts job submissions, serves status, exposes Prometheus `/metrics`. Stateless. |
| **Worker Pool** | Polls Postgres for claimable jobs via `FOR UPDATE SKIP LOCKED`, executes them, sends heartbeats. Horizontally scalable. |
| **Reaper** | Periodic loop that reclaims orphaned jobs (expired leases) and marks dead workers. |
| **PostgreSQL** | The only shared state. All coordination happens through row-level locks and atomic SQL. |
| **Dashboard** | Next.js (App Router, TypeScript, Tailwind) operational console. Polls via SWR every 3 s. |

### Job Lifecycle

```
  enqueue ──▶ queued ──▶ claimed ──▶ running ──┬──▶ completed
                 ▲                             │
                 │         retry (attempts      ├──▶ failed ──▶ retry ──▶ queued
                 │          < max)              │
                 │                              └──▶ dead_letter
                 │
                 └── reaper (lease expired) ────────┘
```

### Entity-Relationship Diagram

See the full Mermaid diagram: [`docs/ER.md`](docs/ER.md)

### Design Document

Detailed write-up covering atomic claims, lease/heartbeat/reaper recovery, idempotency, retry strategies, and trade-offs: [`docs/DESIGN.md`](docs/DESIGN.md)

### API Reference (OpenAPI)

Full OpenAPI 3.0 spec: [`docs/openapi.yaml`](docs/openapi.yaml)

You can import it into Swagger UI, Redoc, or any OpenAPI-compatible tool.

---

## Quick Start (30 seconds)

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose v2

### One-command launch

```bash
docker compose up --build -d
```

This starts **6 services**:

| Service | Port | Description |
|---------|------|-------------|
| `postgres` | 5432 | PostgreSQL 16 |
| `migrate` | — | Applies schema migrations, then exits |
| `api` | **8080** | REST API + Prometheus metrics |
| `worker-1` | — | Worker (concurrency=5) |
| `worker-2` | — | Worker (concurrency=5) |
| `reaper` | — | Lease reaper (10 s interval) |
| `dashboard` | **3000** | Next.js control plane |

### Seed demo data

Once the services are up (wait ~5 s for the API to become healthy):

```bash
./scripts/seed.sh
```

This registers a demo org, creates 3 queues (Emails, Webhooks, Reports), and submits **22 jobs** — a mix of:

- **8 immediate** sleep jobs
- **3 delayed** jobs (10 s, 20 s, 30 s)
- **1 recurring** cron job (every 2 min)
- **3 always-fail** jobs (will exhaust retries → DLQ within ~30 s)
- **1 idempotent** job (submitted twice, second is a no-op)
- **3-job batch** with mixed priorities
- **4 webhook** sleep jobs

### Open the dashboard

```
http://localhost:3000
```

Login: `admin@demo.com` / `demodemo123`

You'll see jobs being processed in real-time, workers heartbeating, and failing jobs moving through retries into the dead-letter queue.

### Tear down

```bash
docker compose down -v
```

---

## Local Development (without Docker)

```bash
# Start Postgres (e.g. via brew or docker)
export DATABASE_URL="postgres://localhost:5432/scheduler?sslmode=disable"

# Apply migrations
MIGRATIONS_DIR=./migrations go run ./cmd/migrate

# Start the API (with embedded reaper for single-node dev)
EMBED_REAPER=true JWT_SECRET=dev-secret go run ./cmd/api

# Start a worker
WORKER_NAME=dev-worker go run ./cmd/worker

# Start the dashboard
cd dashboard && npm install && npm run dev
```

---

## Environment Variables

### API Server (`cmd/api`)

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | `postgres://localhost:5432/scheduler?sslmode=disable` | Postgres connection string |
| `JWT_SECRET` | `change-me-in-production` | HMAC secret for JWT signing |
| `JWT_TTL` | `24h` | Token lifetime (Go duration) |
| `EMBED_REAPER` | `false` | Run the reaper as a goroutine inside the API process |
| `REAPER_INTERVAL` | `10s` | How often the embedded reaper sweeps |
| `STALE_THRESHOLD` | `30s` | Heartbeat staleness threshold for marking workers dead |

### Worker (`cmd/worker`)

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://localhost:5432/scheduler?sslmode=disable` | Postgres connection string |
| `WORKER_NAME` | `$(hostname)` | Human-readable worker name |
| `MAX_CONCURRENCY` | `10` | Max parallel job goroutines |
| `POLL_INTERVAL` | `2s` | Time between claim polls |
| `LEASE_SECONDS` | `30` | Lease duration on claimed jobs |
| `HEARTBEAT_INTERVAL` | `10s` | Heartbeat tick (must be < lease/2) |
| `DRAIN_TIMEOUT` | `30s` | Max wait for in-flight jobs on shutdown |

### Reaper (`cmd/reaper`)

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://localhost:5432/scheduler?sslmode=disable` | Postgres connection string |
| `REAPER_INTERVAL` | `10s` | Sweep interval |
| `STALE_THRESHOLD` | `30s` | Mark workers dead after this heartbeat gap |

### Dashboard (`dashboard/`)

| Variable | Default | Description |
|----------|---------|-------------|
| `NEXT_PUBLIC_API_URL` | `http://localhost:8080` | API base URL for the rewrite proxy |

---

## API Endpoints

Full OpenAPI spec: [`docs/openapi.yaml`](docs/openapi.yaml)

### Auth

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/auth/register` | Register org + admin user, returns JWT |
| `POST` | `/auth/login` | Authenticate, returns JWT |

### Projects

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/projects` | List projects (paginated) |
| `POST` | `/projects` | Create project |
| `GET` | `/projects/{id}` | Get project |
| `PUT` | `/projects/{id}` | Update project |
| `DELETE` | `/projects/{id}` | Delete project |

### Queues

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/queues` | List queues (paginated, filterable) |
| `POST` | `/queues` | Create queue |
| `GET` | `/queues/{id}` | Get queue |
| `PUT` | `/queues/{id}` | Update queue config |
| `DELETE` | `/queues/{id}` | Delete queue |
| `POST` | `/queues/{id}/pause` | Pause queue |
| `POST` | `/queues/{id}/resume` | Resume queue |
| `GET` | `/queues/{id}/stats` | Queue statistics |
| `GET` | `/queues/{id}/jobs` | List jobs in queue |

### Jobs

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/queues/{id}/jobs` | Create job (immediate/delayed/scheduled/recurring) |
| `POST` | `/queues/{id}/jobs/batch` | Create batch of jobs |
| `GET` | `/jobs` | List all jobs (filterable by status, queue) |
| `GET` | `/jobs/{id}` | Get job detail |
| `GET` | `/jobs/{id}/executions` | Execution history |
| `GET` | `/jobs/{id}/logs` | Job log timeline |
| `POST` | `/jobs/{id}/retry` | Retry a dead-lettered job |

### DLQ & Workers

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/dlq` | List dead-lettered jobs |
| `GET` | `/workers` | List active workers |

### System

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Health check |
| `GET` | `/metrics` | Prometheus metrics |

---

## Testing

```bash
# Unit tests (no Docker needed)
make test-unit

# Integration tests (requires Docker for testcontainers)
make test-integration

# Everything
make test
```

The test suite covers 6 critical paths:

1. **Concurrent claim correctness** — N goroutines claim from the same queue; zero duplicates, budget enforced
2. **Retry backoff curves** — fixed, linear, exponential with cap and jitter
3. **DLQ transition** — job exhausts max_attempts, moves to dead_letter, manual retry requeues
4. **Reaper requeues orphaned jobs** — expired lease → back to queued, dead worker → offline
5. **Idempotency conflict** — duplicate `idempotency_key` returns existing job (200, not 201)
6. **Graceful shutdown** — SIGTERM drains in-flight work, releases claims, deregisters worker

---

## Project Structure

```
├── cmd/
│   ├── api/          # API server entrypoint
│   ├── worker/       # Worker entrypoint
│   ├── reaper/       # Reaper entrypoint
│   └── migrate/      # Schema migration tool
├── internal/
│   ├── domain/       # Models, interfaces, errors (leaf package)
│   ├── store/        # pgx repository implementations
│   ├── api/          # HTTP handlers, middleware, routing
│   ├── worker/       # Worker engine, job handlers
│   └── reaper/       # Lease reaper engine
├── pkg/
│   ├── auth/         # JWT + bcrypt
│   ├── retry/        # Backoff calculation
│   ├── metrics/      # Prometheus metrics
│   └── pagination/   # Cursor-based pagination
├── migrations/       # Goose SQL migrations (00001–00009)
├── dashboard/        # Next.js App Router dashboard
├── scripts/
│   └── seed.sh       # Demo data seeder
├── docs/
│   ├── DESIGN.md     # System design document
│   ├── ER.md         # Mermaid ER diagram
│   └── openapi.yaml  # OpenAPI 3.0 specification
├── docker-compose.yml
├── Dockerfile
└── Makefile
```

---

## License

MIT
