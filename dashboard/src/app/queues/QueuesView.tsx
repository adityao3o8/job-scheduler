"use client";

import { useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { mutate } from "swr";
import { Layers, Plus } from "lucide-react";
import { useQueues, useAllQueueStats, useProjects } from "@/lib/use-api";
import { api } from "@/lib/api";
import type { Queue } from "@/lib/types";
import { Drawer } from "@/components/ui/Drawer";
import { Button } from "@/components/ui/Button";
import { StatusChip } from "@/components/ui/StatusChip";
import { EmptyState } from "@/components/ui/EmptyState";
import { TableSkeleton } from "@/components/ui/Skeleton";
import { RelativeTime } from "@/components/ui/CopyId";
import { CreateQueueForm } from "@/components/forms/CreateQueueForm";
import { useConsole } from "@/components/providers/ConsoleProvider";

function QueueDrawer({
  queue,
  depth,
  throughput,
  onClose,
  onUpdated,
}: {
  queue: Queue;
  depth?: number;
  throughput?: number;
  onClose: () => void;
  onUpdated: () => void;
}) {
  const [name, setName] = useState(queue.name);
  const [concurrency, setConcurrency] = useState(queue.concurrency_limit?.toString() ?? "");
  const [priority, setPriority] = useState(String(queue.priority_default));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function save() {
    setSaving(true);
    setError("");
    try {
      await api.updateQueue(queue.id, {
        name,
        priority_default: parseInt(priority, 10) || 0,
        concurrency_limit: concurrency ? parseInt(concurrency, 10) : undefined,
      });
      onUpdated();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Update failed");
    } finally {
      setSaving(false);
    }
  }

  async function togglePause() {
    if (queue.is_paused) await api.resumeQueue(queue.id);
    else await api.pauseQueue(queue.id);
    onUpdated();
    onClose();
  }

  const inFlight = depth ?? 0;
  const limit = queue.concurrency_limit ?? 0;
  const pct = limit > 0 ? Math.min(100, (inFlight / limit) * 100) : 0;

  return (
    <Drawer
      open
      onClose={onClose}
      title={queue.name}
      subtitle={queue.slug}
      actions={
        <Button
          variant={queue.is_paused ? "primary" : "secondary"}
          size="sm"
          onClick={togglePause}
        >
          {queue.is_paused ? "Resume queue" : "Pause queue"}
        </Button>
      }
    >
      <div className="space-y-6">
        <div className="grid grid-cols-2 gap-4">
          <div>
            <p className="eyebrow mb-1">Depth</p>
            <p className="display-xl mono-data">{depth ?? "—"}</p>
          </div>
          <div>
            <p className="eyebrow mb-1">Throughput / 1h</p>
            <p className="display-xl mono-data">{throughput ?? "—"}</p>
          </div>
        </div>

        <div>
          <div className="flex justify-between mb-2">
            <span className="eyebrow">Concurrency</span>
            <span className="mono-data body-sm" style={{ color: "var(--text-tertiary)" }}>
              {limit ? `${inFlight} / ${limit}` : "Unlimited"}
            </span>
          </div>
          {limit > 0 && (
            <div
              className="h-1 overflow-hidden"
              style={{ background: "var(--bg-void)", borderRadius: "var(--r-sm)" }}
            >
              <div
                style={{
                  width: `${pct}%`,
                  height: "100%",
                  background: "var(--accent)",
                  transition: "width 150ms ease",
                }}
              />
            </div>
          )}
        </div>

        <div>
          <p className="eyebrow mb-2">Recent activity</p>
          <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
            Updated <RelativeTime ts={queue.updated_at} />. Poll interval 3s.
          </p>
        </div>

        {error && (
          <p className="body-sm" style={{ color: "var(--st-failed)" }}>
            {error}
          </p>
        )}

        <div className="space-y-4 pt-2">
          <p className="eyebrow">Configuration</p>
          <label className="block">
            <span className="eyebrow mb-2 block">Name</span>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="block">
            <span className="eyebrow mb-2 block">Concurrency limit</span>
            <input
              className="input mono-data"
              value={concurrency}
              onChange={(e) => setConcurrency(e.target.value)}
              placeholder="Unlimited"
            />
          </label>
          <label className="block">
            <span className="eyebrow mb-2 block">Default priority</span>
            <input
              className="input mono-data"
              value={priority}
              onChange={(e) => setPriority(e.target.value)}
            />
          </label>
          <Button onClick={save} disabled={saving}>
            {saving ? "Saving…" : "Save configuration"}
          </Button>
        </div>
      </div>
    </Drawer>
  );
}

export default function QueuesView() {
  const searchParams = useSearchParams();
  const { createQueueRequest } = useConsole();
  const { data: queuePage, isLoading, error } = useQueues();
  const { data: projectPage } = useProjects();
  const queues = queuePage?.items ?? [];
  const projects = projectPage?.items ?? [];
  const queueIds = queues.map((q) => q.id);
  const { data: stats } = useAllQueueStats(queueIds);
  const [selected, setSelected] = useState<Queue | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  useEffect(() => {
    if (searchParams.get("create") === "1") setCreateOpen(true);
  }, [searchParams]);

  useEffect(() => {
    if (createQueueRequest > 0) setCreateOpen(true);
  }, [createQueueRequest]);

  if (error) {
    return (
      <div className="panel p-6" style={{ borderColor: "var(--st-failed)" }}>
        <p className="h2" style={{ color: "var(--st-failed)" }}>
          Failed to load queues
        </p>
        <p className="body-sm mt-2" style={{ color: "var(--text-tertiary)" }}>
          {error.message}. Check API connectivity and sign in again.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4 panel-enter">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
          Click a row to inspect configuration and pause or resume processing.
        </p>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus size={14} className="inline mr-2 -mt-0.5" />
          Create queue
        </Button>
      </div>

      {isLoading ? (
        <TableSkeleton rows={4} />
      ) : queues.length === 0 ? (
        <EmptyState
          icon={Layers}
          title="No queues"
          description="Queues isolate workloads and control concurrency. Create one to start submitting jobs."
          action={
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              Create queue
            </Button>
          }
        />
      ) : (
        <div className="panel overflow-hidden overflow-x-auto">
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th className="numeric">Depth</th>
                <th>Concurrency</th>
                <th className="numeric">/ 1h</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {queues.map((q) => {
                const s = stats?.find((x) => x.queue_id === q.id);
                const limit = q.concurrency_limit ?? 0;
                const depth = s?.depth ?? 0;
                const pct = limit > 0 ? Math.min(100, (depth / limit) * 100) : 0;
                return (
                  <tr
                    key={q.id}
                    className="clickable"
                    onClick={() => setSelected(q)}
                  >
                    <td>
                      <span className="body-sm" style={{ color: "var(--text-primary)", fontWeight: 500 }}>
                        {q.name}
                      </span>
                      <span
                        className="block mono-data"
                        style={{ color: "var(--text-tertiary)", fontSize: 11 }}
                      >
                        {q.slug}
                      </span>
                    </td>
                    <td className="numeric">{depth}</td>
                    <td>
                      <div className="flex items-center gap-2 min-w-[100px]">
                        <div
                          className="flex-1 h-1 overflow-hidden"
                          style={{
                            background: "var(--bg-void)",
                            borderRadius: "var(--r-sm)",
                            maxWidth: 80,
                          }}
                        >
                          <div
                            style={{
                              width: limit ? `${pct}%` : "0%",
                              height: "100%",
                              background: "var(--accent)",
                            }}
                          />
                        </div>
                        <span className="mono-data body-sm" style={{ color: "var(--text-tertiary)" }}>
                          {limit ? `${depth}/${limit}` : "∞"}
                        </span>
                      </div>
                    </td>
                    <td className="numeric">{s?.throughput_1h ?? "—"}</td>
                    <td>
                      <StatusChip status={q.is_paused ? "paused" : "active"} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {selected && (
        <QueueDrawer
          queue={selected}
          depth={stats?.find((s) => s.queue_id === selected.id)?.depth}
          throughput={stats?.find((s) => s.queue_id === selected.id)?.throughput_1h}
          onClose={() => setSelected(null)}
          onUpdated={() => mutate("queues")}
        />
      )}

      <Drawer open={createOpen} onClose={() => setCreateOpen(false)} title="Create queue">
        <CreateQueueForm
          projects={projects}
          onCreated={() => {
            setCreateOpen(false);
            mutate("queues");
          }}
          onCancel={() => setCreateOpen(false)}
        />
      </Drawer>
    </div>
  );
}
