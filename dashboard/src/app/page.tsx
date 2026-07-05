"use client";

import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { useQueues, useAllQueueStats, useWorkers } from "@/lib/use-api";
import { useConsole } from "@/components/providers/ConsoleProvider";
import { Pulse } from "@/components/ui/Pulse";
import { StatPanel } from "@/components/ui/StatPanel";
import { StatusChip } from "@/components/ui/StatusChip";
import { RelativeTime } from "@/components/ui/CopyId";
import { formatPercent } from "@/lib/format";
import Link from "next/link";

export default function OverviewPage() {
  const { pulseEvents } = useConsole();
  const { data: queuePage, isLoading: queuesLoading } = useQueues();
  const queues = queuePage?.items ?? [];
  const queueIds = queues.map((q) => q.id);
  const { data: stats, isLoading: statsLoading } = useAllQueueStats(queueIds);
  const { data: workers, isLoading: workersLoading } = useWorkers();

  const loading = queuesLoading || statsLoading;

  const totalDepth = stats?.reduce((a, s) => a + s.depth, 0) ?? 0;
  const throughput1h = stats?.reduce((a, s) => a + s.throughput_1h, 0) ?? 0;
  const throughput24h = stats?.reduce((a, s) => a + s.throughput_24h, 0) ?? 0;
  const avgSuccess =
    stats?.length ? stats.reduce((a, s) => a + s.success_rate, 0) / stats.length : 0;
  const activeWorkers = workers?.filter((w) => w.status === "active").length ?? 0;

  const chartData =
    stats?.map((s) => ({
      name: s.queue_name.length > 10 ? s.queue_name.slice(0, 10) + "…" : s.queue_name,
      completed: Math.round(s.throughput_1h * s.success_rate),
      failed: Math.round(s.throughput_1h * (1 - s.success_rate)),
      depth: s.depth,
    })) ?? [];

  return (
    <div className="space-y-6">
      <section className="panel-enter panel-enter-1">
        <div className="px-4 pt-4 pb-2 flex items-center justify-between gap-4">
          <div>
            <p className="eyebrow mb-1">System pulse</p>
            <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
              Live trace of job completions and failures
            </p>
          </div>
        </div>
        <div className="px-4 pb-4">
          <Pulse events={pulseEvents} />
        </div>
      </section>

      <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <StatPanel
          className="panel-enter panel-enter-2"
          label="Queue depth"
          value={totalDepth}
          loading={loading}
          sparkline={stats?.map((s) => s.depth)}
          sparkColor="var(--accent)"
          delta="Jobs waiting to run"
        />
        <StatPanel
          className="panel-enter panel-enter-3"
          label="Throughput / 1h"
          value={throughput1h}
          loading={loading}
          sparkline={stats?.map((s) => s.throughput_1h)}
          sparkColor="var(--st-running)"
        />
        <StatPanel
          className="panel-enter panel-enter-4"
          label="Throughput / 24h"
          value={throughput24h}
          loading={loading}
          sparkline={stats?.map((s) => s.throughput_24h)}
          sparkColor="var(--st-scheduled)"
        />
        <StatPanel
          className="panel-enter panel-enter-5"
          label="Success rate"
          value={loading ? "—" : formatPercent(avgSuccess)}
          loading={loading}
          deltaPositive={avgSuccess >= 0.9}
          delta={avgSuccess >= 0.95 ? "Within SLO" : avgSuccess >= 0.8 ? "Below target" : "Investigate failures"}
          sparkColor="var(--st-completed)"
        />
        <StatPanel
          className="panel-enter panel-enter-6"
          label="Active workers"
          value={activeWorkers}
          loading={workersLoading}
          sparkColor="var(--st-completed)"
        />
      </div>

      {chartData.length > 0 && (
        <section className="panel p-4 panel-enter panel-enter-4">
          <p className="eyebrow mb-4">Throughput by queue</p>
          <div style={{ height: 240 }}>
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={chartData} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="gCompleted" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#43C98A" stopOpacity={0.15} />
                    <stop offset="100%" stopColor="#43C98A" stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="gFailed" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#F26759" stopOpacity={0.15} />
                    <stop offset="100%" stopColor="#F26759" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid stroke="var(--border-hairline)" strokeDasharray="3 3" vertical={false} />
                <XAxis
                  dataKey="name"
                  tick={{ fill: "var(--text-tertiary)", fontSize: 11, fontFamily: "var(--font-geist-mono)" }}
                  axisLine={{ stroke: "var(--border-hairline)" }}
                  tickLine={false}
                />
                <YAxis
                  tick={{ fill: "var(--text-tertiary)", fontSize: 11, fontFamily: "var(--font-geist-mono)" }}
                  axisLine={false}
                  tickLine={false}
                />
                <Tooltip
                  contentStyle={{
                    background: "var(--bg-panel)",
                    border: "1px solid var(--border-hairline)",
                    borderRadius: "var(--r-md)",
                    fontSize: 12,
                    fontFamily: "var(--font-geist-mono)",
                  }}
                />
                <Area
                  type="monotone"
                  dataKey="completed"
                  stroke="var(--st-completed)"
                  fill="url(#gCompleted)"
                  strokeWidth={1.5}
                  name="Completed"
                />
                <Area
                  type="monotone"
                  dataKey="failed"
                  stroke="var(--st-failed)"
                  fill="url(#gFailed)"
                  strokeWidth={1.5}
                  name="Failed"
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </section>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <section className="panel panel-enter panel-enter-5">
          <div className="px-4 py-3 flex items-center justify-between" style={{ borderBottom: "1px solid var(--border-hairline)" }}>
            <p className="eyebrow">Queues at a glance</p>
            <Link href="/queues" className="body-sm" style={{ color: "var(--accent)" }}>
              View all
            </Link>
          </div>
          <div className="overflow-x-auto">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Queue</th>
                  <th className="numeric">Depth</th>
                  <th className="numeric">/ 1h</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {stats?.map((s) => {
                  const q = queues.find((x) => x.id === s.queue_id);
                  return (
                    <tr key={s.queue_id} className="clickable" onClick={() => (window.location.href = "/queues")}>
                      <td className="body-sm" style={{ color: "var(--text-primary)" }}>
                        {s.queue_name}
                      </td>
                      <td className="numeric">{s.depth}</td>
                      <td className="numeric">{s.throughput_1h}</td>
                      <td>
                        <StatusChip status={q?.is_paused ? "paused" : "active"} />
                      </td>
                    </tr>
                  );
                })}
                {!loading && (!stats || stats.length === 0) && (
                  <tr>
                    <td colSpan={4} className="text-center py-8" style={{ color: "var(--text-tertiary)" }}>
                      No queues yet. Create one to start scheduling work.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        <section className="panel panel-enter panel-enter-6">
          <div className="px-4 py-3 flex items-center justify-between" style={{ borderBottom: "1px solid var(--border-hairline)" }}>
            <p className="eyebrow">Workers</p>
            <Link href="/workers" className="body-sm" style={{ color: "var(--accent)" }}>
              View all
            </Link>
          </div>
          <div className="p-4 space-y-3">
            {workers?.slice(0, 6).map((w) => (
              <div
                key={w.id}
                className="flex items-center justify-between gap-4 py-2"
                style={{ borderBottom: "1px solid var(--border-hairline)" }}
              >
                <div>
                  <p className="body-sm" style={{ color: "var(--text-primary)", fontWeight: 500 }}>
                    {w.name}
                  </p>
                  <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
                    {w.last_seen_at ? <RelativeTime ts={w.last_seen_at} /> : "No heartbeat"}
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <span className="mono-data body-sm" style={{ color: "var(--accent)" }}>
                    {w.jobs_in_flight} in flight
                  </span>
                  <StatusChip status={w.status} pulse={w.status === "active"} />
                </div>
              </div>
            ))}
            {!workersLoading && (!workers || workers.length === 0) && (
              <p className="body-sm py-6 text-center" style={{ color: "var(--text-tertiary)" }}>
                No workers registered. Start a worker process to claim jobs.
              </p>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
