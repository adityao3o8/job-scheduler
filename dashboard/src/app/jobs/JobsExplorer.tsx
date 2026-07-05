"use client";

import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import { ListTodo, Plus } from "lucide-react";
import { useJobs, useQueues } from "@/lib/use-api";
import { JOB_STATUSES, getStatusStyle } from "@/lib/status";
import { StatusChip } from "@/components/ui/StatusChip";
import { Drawer } from "@/components/ui/Drawer";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { TableSkeleton } from "@/components/ui/Skeleton";
import { CopyId, RelativeTime } from "@/components/ui/CopyId";
import { JobDetailContent } from "@/components/jobs/JobDetailContent";
import { SubmitJobForm } from "@/components/forms/SubmitJobForm";
import { jobDisplayName } from "@/lib/format";
import { useConsole } from "@/components/providers/ConsoleProvider";

export default function JobsExplorer() {
  const searchParams = useSearchParams();
  const { submitJobRequest } = useConsole();
  const [statusFilters, setStatusFilters] = useState<string[]>([]);
  const [queueId, setQueueId] = useState("");
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null);
  const [submitOpen, setSubmitOpen] = useState(false);

  const { data: queuePage } = useQueues();
  const queues = queuePage?.items ?? [];

  const params = useMemo(() => {
    const p: Record<string, string> = {};
    if (statusFilters.length === 1) p.status = statusFilters[0];
    if (queueId) p.queue_id = queueId;
    return p;
  }, [statusFilters, queueId]);

  const { data: jobPage, isLoading, error } = useJobs(params);
  const jobs = useMemo(() => {
    const items = jobPage?.items ?? [];
    if (statusFilters.length <= 1) return items;
    return items.filter((j) => statusFilters.includes(j.status));
  }, [jobPage, statusFilters]);

  useEffect(() => {
    const id = searchParams.get("job");
    if (id) setSelectedJobId(id);
    if (searchParams.get("submit") === "1") setSubmitOpen(true);
  }, [searchParams]);

  useEffect(() => {
    if (submitJobRequest > 0) setSubmitOpen(true);
  }, [submitJobRequest]);

  function toggleStatus(s: string) {
    setStatusFilters((prev) =>
      prev.includes(s) ? prev.filter((x) => x !== s) : [...prev, s]
    );
  }

  if (error) {
    return (
      <div className="panel p-6" style={{ borderColor: "var(--st-failed)" }}>
        <p className="h2" style={{ color: "var(--st-failed)" }}>
          Failed to load jobs
        </p>
        <p className="body-sm mt-2" style={{ color: "var(--text-tertiary)" }}>
          {error.message}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4 panel-enter">
      <div
        className="panel px-4 py-3 flex flex-col sm:flex-row sm:items-center justify-between gap-3 sticky z-10"
        style={{ top: 0 }}
      >
        <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
          Filter jobs by status and queue. Open a row for execution history and logs.
        </p>
        <Button size="sm" onClick={() => setSubmitOpen(true)} className="w-full sm:w-auto">
          <Plus size={14} />
          Submit job
        </Button>
      </div>

      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap gap-2">
          {JOB_STATUSES.map((s) => {
            const st = getStatusStyle(s);
            const active = statusFilters.includes(s);
            return (
              <button
                key={s}
                type="button"
                onClick={() => toggleStatus(s)}
                className="status-chip transition-opacity"
                style={{
                  background: active ? st.dim : "transparent",
                  border: `1px solid ${active ? st.color : "var(--border-strong)"}`,
                  color: active ? st.color : "var(--text-tertiary)",
                  cursor: "pointer",
                }}
              >
                <span className="status-dot" style={{ background: st.color }} />
                {st.label}
              </button>
            );
          })}
        </div>

        <select
          value={queueId}
          onChange={(e) => setQueueId(e.target.value)}
          className="input mono-data max-w-xs"
          aria-label="Filter by queue"
        >
          <option value="">All queues</option>
          {queues.map((q) => (
            <option key={q.id} value={q.id}>
              {q.name}
            </option>
          ))}
        </select>
      </div>

      {isLoading ? (
        <TableSkeleton rows={8} />
      ) : jobs.length === 0 ? (
        <EmptyState
          icon={ListTodo}
          title="No jobs match"
          description="Adjust filters or submit work to a queue. Jobs appear here as soon as they are enqueued."
          action={
            <Button size="sm" onClick={() => setSubmitOpen(true)}>
              Submit job
            </Button>
          }
        />
      ) : (
        <div className="panel overflow-x-auto">
          <table className="data-table">
            <thead>
              <tr>
                <th>Job</th>
                <th>Status</th>
                <th>Queue</th>
                <th className="numeric">Priority</th>
                <th className="numeric">Attempts</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((j) => {
                const q = queues.find((x) => x.id === j.queue_id);
                const name = jobDisplayName(j);
                return (
                  <tr
                    key={j.id}
                    className="clickable"
                    onClick={() => setSelectedJobId(j.id)}
                  >
                    <td>
                      <span className="body-sm" style={{ color: "var(--text-primary)" }}>
                        {name}
                      </span>
                      <span
                        className="block mono-data"
                        style={{ color: "var(--text-tertiary)", fontSize: 11 }}
                      >
                        <CopyId id={j.id} display={j.id} />
                      </span>
                    </td>
                    <td>
                      <StatusChip status={j.status} pulse={j.status === "running"} />
                    </td>
                    <td className="body-sm" style={{ color: "var(--text-secondary)" }}>
                      {q?.name ?? "Unknown queue"}
                    </td>
                    <td className="numeric">{j.priority}</td>
                    <td className="numeric">
                      {j.attempts}/{j.max_attempts}
                    </td>
                    <td>
                      <RelativeTime ts={j.created_at} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {jobPage?.has_more && (
        <p className="body-sm mono-data" style={{ color: "var(--text-tertiary)" }}>
          More results available — narrow filters to refine.
        </p>
      )}

      <Drawer
        open={!!selectedJobId}
        onClose={() => setSelectedJobId(null)}
        title={
          selectedJobId
            ? jobDisplayName(jobs.find((j) => j.id === selectedJobId) ?? {})
            : "Job detail"
        }
        subtitle={
          selectedJobId
            ? queues.find((q) => q.id === jobs.find((j) => j.id === selectedJobId)?.queue_id)?.name
            : undefined
        }
      >
        {selectedJobId && <JobDetailContent jobId={selectedJobId} />}
      </Drawer>

      <Drawer open={submitOpen} onClose={() => setSubmitOpen(false)} title="Submit job">
        <SubmitJobForm
          queues={queues}
          defaultQueueId={queueId || undefined}
          onSubmitted={(jobId) => {
            setSubmitOpen(false);
            setSelectedJobId(jobId);
          }}
          onCancel={() => setSubmitOpen(false)}
        />
      </Drawer>
    </div>
  );
}
