"use client";

import { usePathname } from "next/navigation";
import { ChevronRight, Plus, Search } from "lucide-react";
import { Pulse } from "@/components/ui/Pulse";
import { Button } from "@/components/ui/Button";
import { useConsole } from "@/components/providers/ConsoleProvider";
import { useQueues, useWorkers } from "@/lib/use-api";

const BREADCRUMBS: Record<string, string> = {
  "/": "Overview",
  "/queues": "Queues",
  "/jobs": "Jobs",
  "/workers": "Workers",
  "/dlq": "Dead Letter",
};

export function TopBar() {
  const path = usePathname();
  const { pulseEvents, openCommand, openSubmitJob, openCreateQueue } = useConsole();
  const { data: queuePage } = useQueues();
  const { data: workers } = useWorkers();

  const queues = queuePage?.items ?? [];
  const pausedCount = queues.filter((q) => q.is_paused).length;
  const activeWorkers = workers?.filter((w) => w.status === "active").length ?? 0;
  const degraded = pausedCount > 0 || activeWorkers === 0;

  let crumb = BREADCRUMBS[path] ?? "Overview";
  if (path.startsWith("/jobs/")) crumb = "Job detail";

  return (
    <header
      className="flex items-center gap-4 px-6 shrink-0"
      style={{
        height: "var(--topbar-height)",
        background: "var(--bg-panel)",
        borderBottom: "1px solid var(--border-hairline)",
      }}
    >
      <nav className="flex items-center gap-2 min-w-0 flex-1" aria-label="Breadcrumb">
        <span className="eyebrow hidden sm:inline">Console</span>
        <ChevronRight size={14} className="hidden sm:inline" style={{ color: "var(--text-tertiary)" }} />
        <span className="h2 truncate">{crumb}</span>
      </nav>

      {(path === "/jobs" || path.startsWith("/jobs/")) && (
        <Button size="sm" onClick={openSubmitJob} className="shrink-0">
          <Plus size={14} />
          Submit job
        </Button>
      )}
      {path === "/queues" && (
        <Button size="sm" onClick={openCreateQueue} className="shrink-0">
          <Plus size={14} />
          Create queue
        </Button>
      )}

      <button
        type="button"
        onClick={openCommand}
        className="hidden sm:flex items-center gap-2 px-3 py-1.5 transition-colors"
        style={{
          background: "var(--bg-surface)",
          border: "1px solid var(--border-strong)",
          borderRadius: "var(--r-md)",
          color: "var(--text-tertiary)",
        }}
      >
        <Search size={14} />
        <span className="body-sm">Search</span>
        <kbd className="eyebrow ml-2">⌘K</kbd>
      </button>

      <div
        className="hidden lg:flex items-center gap-2 px-3 py-1.5"
        style={{
          background: degraded ? "var(--st-retrying-dim)" : "var(--st-completed-dim)",
          border: `1px solid ${degraded ? "var(--st-retrying)" : "var(--st-completed)"}`,
          borderRadius: "var(--r-md)",
        }}
      >
        <span
          className={`status-dot ${!degraded ? "pulse" : ""}`}
          style={{
            width: 8,
            height: 8,
            background: degraded ? "var(--st-retrying)" : "var(--st-completed)",
          }}
        />
        <span className="body-sm" style={{ color: "var(--text-secondary)" }}>
          {degraded ? "Degraded" : "Operational"}
        </span>
      </div>

      <Pulse events={pulseEvents} compact className="hidden md:block shrink-0" />
    </header>
  );
}
