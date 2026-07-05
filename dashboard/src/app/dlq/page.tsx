"use client";

import { useState } from "react";
import { mutate } from "swr";
import { Skull } from "lucide-react";
import { useDLQ } from "@/lib/use-api";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { RelativeTime } from "@/components/ui/CopyId";
import { jobDisplayName } from "@/lib/format";

export default function DLQPage() {
  const { data: dlqPage, isLoading, error } = useDLQ();
  const items = dlqPage?.items ?? [];
  const [retrying, setRetrying] = useState<string | null>(null);
  const [actionError, setActionError] = useState("");

  async function retry(jobId: string) {
    setRetrying(jobId);
    setActionError("");
    try {
      await api.retryJob(jobId);
      mutate("dlq");
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Retry failed");
    } finally {
      setRetrying(null);
    }
  }

  if (error) {
    return (
      <div className="panel p-6" style={{ borderColor: "var(--st-failed)" }}>
        <p className="h2" style={{ color: "var(--st-failed)" }}>
          Failed to load dead letter queue
        </p>
        <p className="body-sm mt-2" style={{ color: "var(--text-tertiary)" }}>
          {error.message}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4 panel-enter">
      <div className="flex items-center justify-between gap-4">
        <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
          Jobs that exhausted all retry attempts. Inspect the error, then retry or discard.
        </p>
        <span className="mono-data body-sm" style={{ color: "var(--text-tertiary)" }}>
          {items.length} item{items.length !== 1 ? "s" : ""}
        </span>
      </div>

      {actionError && (
        <p className="body-sm panel p-3" style={{ color: "var(--st-failed)", borderColor: "var(--st-failed)" }}>
          {actionError}
        </p>
      )}

      {isLoading ? (
        <div className="space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="panel p-4">
              <div className="skeleton h-4 w-32 mb-2" />
              <div className="skeleton h-16 w-full" />
            </div>
          ))}
        </div>
      ) : items.length === 0 ? (
        <EmptyState
          icon={Skull}
          title="Dead letter queue is empty"
          description="No dead-lettered jobs. Failures that exhaust retries land here for operator review."
        />
      ) : (
        <div className="space-y-4">
          {items.map((d) => (
            <article
              key={d.id}
              className="panel p-4"
              style={{
                background: "var(--st-dead-dim)",
                borderColor: "var(--st-dead)",
              }}
            >
              <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4">
                <div className="flex-1 min-w-0 space-y-3">
                  <div className="flex flex-wrap items-center gap-3">
                    <span className="body-sm" style={{ color: "var(--text-primary)", fontWeight: 600 }}>
                      {jobDisplayName({ payload: d.payload })}
                    </span>
                    <span className="body-sm" style={{ color: "var(--text-tertiary)" }}>
                      Queue <span style={{ color: "var(--text-secondary)" }}>{d.queue_name}</span>
                    </span>
                  </div>

                  {d.reason && (
                    <pre
                      className="panel-inset p-3 mono-data body-sm overflow-x-auto whitespace-pre-wrap"
                      style={{
                        color: "var(--st-failed)",
                        background: "var(--bg-void)",
                        margin: 0,
                      }}
                    >
                      {d.reason}
                    </pre>
                  )}

                  <div className="flex flex-wrap gap-4 body-sm" style={{ color: "var(--text-tertiary)" }}>
                    <span>
                      Attempts{" "}
                      <span className="mono-data" style={{ color: "var(--text-secondary)" }}>
                        {d.attempts_made}
                      </span>
                    </span>
                    <span>
                      Failed <RelativeTime ts={d.failed_at} />
                    </span>
                  </div>

                  <details>
                    <summary className="eyebrow cursor-pointer select-none">Payload</summary>
                    <pre
                      className="mt-2 mono-data panel-inset p-3 overflow-x-auto max-h-32"
                      style={{ fontSize: 11, color: "var(--text-tertiary)", margin: 0 }}
                    >
                      {JSON.stringify(d.payload, null, 2)}
                    </pre>
                  </details>
                </div>

                <div className="flex gap-2 shrink-0">
                  <Button
                    size="sm"
                    onClick={() => retry(d.job_id)}
                    disabled={retrying === d.job_id}
                  >
                    {retrying === d.job_id ? "Retrying…" : "Retry job"}
                  </Button>
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled
                    title="Discard requires API support"
                  >
                    Discard
                  </Button>
                </div>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}
