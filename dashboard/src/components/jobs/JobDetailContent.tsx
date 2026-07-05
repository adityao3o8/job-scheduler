"use client";

import { StatusChip } from "@/components/ui/StatusChip";
import { CopyId, RelativeTime } from "@/components/ui/CopyId";
import { Skeleton } from "@/components/ui/Skeleton";
import { useJob, useJobExecutions, useJobLogs, useQueues, useWorkers } from "@/lib/use-api";
import { formatDuration, jobDisplayName, queueName, workerName } from "@/lib/format";
import { getStatusStyle } from "@/lib/status";

export function JobDetailContent({ jobId }: { jobId: string }) {
  const { data: job, error: jobError, isLoading } = useJob(jobId);
  const { data: executions } = useJobExecutions(jobId);
  const { data: logs } = useJobLogs(jobId);
  const { data: queuePage } = useQueues();
  const { data: workers } = useWorkers();
  const queues = queuePage?.items ?? [];

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }

  if (jobError || !job) {
    return (
      <div className="panel p-6" style={{ borderColor: "var(--st-failed)" }}>
        <p className="h2 mb-2" style={{ color: "var(--st-failed)" }}>
          Could not load job
        </p>
        <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
          {jobError?.message ?? "Job not found or access denied. Verify the job ID and try again."}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="panel p-4">
        <div className="flex flex-wrap items-center gap-3 mb-4">
          <span className="h2">{jobDisplayName(job)}</span>
          <StatusChip status={job.status} pulse={job.status === "running"} />
        </div>
        <dl className="grid grid-cols-2 gap-4">
          <div>
            <dt className="eyebrow mb-1">Job ID</dt>
            <dd>
              <CopyId id={job.id} display={job.id} />
            </dd>
          </div>
          <div>
            <dt className="eyebrow mb-1">Queue</dt>
            <dd className="body-sm" style={{ color: "var(--text-primary)" }}>
              {queueName(job.queue_id, queues)}
            </dd>
          </div>
          {[
            ["Priority", String(job.priority)],
            ["Attempts", `${job.attempts} / ${job.max_attempts}`],
            ["Created", null],
          ].map(([k, v]) => (
            <div key={k as string}>
              <dt className="eyebrow mb-1">{k}</dt>
              <dd className="mono-data body-sm" style={{ color: "var(--text-primary)" }}>
                {k === "Created" ? <RelativeTime ts={job.created_at} /> : v}
              </dd>
            </div>
          ))}
          {job.cron_expr && (
            <div>
              <dt className="eyebrow mb-1">Cron</dt>
              <dd className="mono-data body-sm">{job.cron_expr}</dd>
            </div>
          )}
          {job.error_message && (
            <div className="col-span-2">
              <dt className="eyebrow mb-1">Last error</dt>
              <dd className="mono-data body-sm" style={{ color: "var(--st-failed)" }}>
                {job.error_message}
              </dd>
            </div>
          )}
        </dl>
      </div>

      <details className="panel">
        <summary className="eyebrow px-4 py-3 cursor-pointer select-none">Payload</summary>
        <pre
          className="px-4 pb-4 mono-data body-sm overflow-x-auto"
          style={{ color: "var(--text-secondary)", margin: 0 }}
        >
          {JSON.stringify(job.payload, null, 2)}
        </pre>
      </details>

      <section>
        <h3 className="eyebrow mb-3">Execution timeline</h3>
        {executions && executions.length > 0 ? (
          <ol className="relative pl-4 space-y-4" style={{ borderLeft: "1px solid var(--border-hairline)" }}>
            {executions.map((e) => {
              const st = getStatusStyle(e.status);
              return (
                <li key={e.id} className="relative pl-4">
                  <span
                    className="absolute -left-[5px] top-1.5 w-2 h-2 rounded-full"
                    style={{ background: st.color }}
                  />
                  <div className="flex flex-wrap items-center gap-2 mb-1">
                    <span className="mono-data body-sm" style={{ color: "var(--text-primary)" }}>
                      Attempt #{e.attempt}
                    </span>
                    <StatusChip status={e.status} />
                    <span className="mono-data body-sm" style={{ color: "var(--text-tertiary)" }}>
                      {formatDuration(e.duration_ms)}
                    </span>
                  </div>
                  <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
                    Worker {workerName(e.worker_id, workers ?? [])} ·{" "}
                    <RelativeTime ts={e.started_at} />
                  </p>
                  {e.error && (
                    <p className="mono-data body-sm mt-1" style={{ color: "var(--st-failed)" }}>
                      {e.error}
                    </p>
                  )}
                </li>
              );
            })}
          </ol>
        ) : (
          <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
            No executions recorded yet.
          </p>
        )}
      </section>

      <section>
        <h3 className="eyebrow mb-3">Logs</h3>
        <div
          className="panel-inset p-3 max-h-64 overflow-y-auto font-mono text-[12px] leading-relaxed"
          style={{ background: "var(--bg-void)" }}
        >
          {logs && logs.length > 0 ? (
            logs.map((l) => (
              <div key={l.id} className="flex gap-3 py-0.5">
                <span style={{ color: "var(--text-tertiary)", minWidth: 72 }}>
                  {new Date(l.created_at).toLocaleTimeString()}
                </span>
                <span
                  style={{
                    color:
                      l.level === "error"
                        ? "var(--st-failed)"
                        : l.level === "warn"
                          ? "var(--st-retrying)"
                          : "var(--text-tertiary)",
                    minWidth: 48,
                  }}
                >
                  [{l.level}]
                </span>
                <span style={{ color: "var(--text-secondary)" }}>{l.message}</span>
              </div>
            ))
          ) : (
            <p style={{ color: "var(--text-tertiary)" }}>No log lines yet.</p>
          )}
        </div>
      </section>
    </div>
  );
}
