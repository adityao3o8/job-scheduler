"use client";

import { Server } from "lucide-react";
import { useWorkers } from "@/lib/use-api";
import { StatusChip } from "@/components/ui/StatusChip";
import { EmptyState } from "@/components/ui/EmptyState";
import { TableSkeleton } from "@/components/ui/Skeleton";
import { RelativeTime } from "@/components/ui/CopyId";

export default function WorkersPage() {
  const { data: workers, isLoading, error } = useWorkers();

  if (error) {
    return (
      <div className="panel p-6" style={{ borderColor: "var(--st-failed)" }}>
        <p className="h2" style={{ color: "var(--st-failed)" }}>
          Failed to load workers
        </p>
        <p className="body-sm mt-2" style={{ color: "var(--text-tertiary)" }}>
          {error.message}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4 panel-enter">
      <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
        Worker heartbeats refresh every 3s. Stale workers are marked offline by the reaper.
      </p>

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="panel p-4">
              <div className="skeleton h-4 w-24 mb-3" />
              <div className="skeleton h-8 w-16" />
            </div>
          ))}
        </div>
      ) : !workers?.length ? (
        <EmptyState
          icon={Server}
          title="No workers registered"
          description="Start a worker process to claim and execute jobs. Workers appear here once they connect."
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {workers.map((w) => (
            <article key={w.id} className="panel p-4 flex flex-col gap-4">
              <div className="flex items-start justify-between gap-2">
                <div>
                  <p className="body-sm" style={{ color: "var(--text-primary)", fontWeight: 600 }}>
                    {w.name}
                  </p>
                </div>
                <StatusChip status={w.status} pulse={w.status === "active"} />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="eyebrow mb-1">In flight</p>
                  <p
                    className="display-xl mono-data"
                    style={{ color: w.jobs_in_flight > 0 ? "var(--accent)" : "var(--text-tertiary)" }}
                  >
                    {w.jobs_in_flight}
                  </p>
                </div>
                <div>
                  <p className="eyebrow mb-1">Last heartbeat</p>
                  <p className="body-sm" style={{ color: "var(--text-secondary)" }}>
                    {w.last_seen_at ? <RelativeTime ts={w.last_seen_at} /> : "—"}
                  </p>
                </div>
              </div>

              <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
                Registered <RelativeTime ts={w.created_at} />
              </p>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}
