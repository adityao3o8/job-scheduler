"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Activity,
  Layers,
  ListTodo,
  Server,
  Skull,
  Pause,
  Play,
  RotateCcw,
  Search,
} from "lucide-react";
import { useConsole } from "@/components/providers/ConsoleProvider";
import { useQueues, useDLQ } from "@/lib/use-api";
import { api } from "@/lib/api";
import { mutate } from "swr";
import { jobDisplayName } from "@/lib/format";

interface CommandItem {
  id: string;
  label: string;
  section: string;
  icon: React.ReactNode;
  keywords: string;
  run: () => void | Promise<void>;
}

function score(query: string, item: CommandItem): number {
  const q = query.toLowerCase().trim();
  if (!q) return 1;
  const hay = `${item.label} ${item.keywords} ${item.section}`.toLowerCase();
  if (hay.startsWith(q)) return 100;
  if (hay.includes(q)) return 50;
  let s = 0;
  for (const word of q.split(/\s+/)) {
    if (hay.includes(word)) s += 10;
  }
  return s;
}

export function CommandPalette() {
  const { commandOpen, setCommandOpen } = useConsole();
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const { data: queuePage } = useQueues();
  const { data: dlqPage } = useDLQ();
  const queues = queuePage?.items ?? [];
  const dlq = dlqPage?.items ?? [];

  const items = useMemo<CommandItem[]>(() => {
    const nav: CommandItem[] = [
      { id: "nav-overview", label: "Go to Overview", section: "Navigate", icon: <Activity size={16} />, keywords: "home dashboard", run: () => router.push("/") },
      { id: "nav-queues", label: "Go to Queues", section: "Navigate", icon: <Layers size={16} />, keywords: "queues", run: () => router.push("/queues") },
      { id: "nav-jobs", label: "Go to Jobs", section: "Navigate", icon: <ListTodo size={16} />, keywords: "jobs explorer", run: () => router.push("/jobs") },
      { id: "nav-workers", label: "Go to Workers", section: "Navigate", icon: <Server size={16} />, keywords: "workers", run: () => router.push("/workers") },
      { id: "nav-dlq", label: "Go to Dead Letter", section: "Navigate", icon: <Skull size={16} />, keywords: "dlq dead letter", run: () => router.push("/dlq") },
      { id: "action-submit-job", label: "Submit job", section: "Actions", icon: <ListTodo size={16} />, keywords: "enqueue create add job", run: () => router.push("/jobs?submit=1") },
      { id: "action-create-queue", label: "Create queue", section: "Actions", icon: <Layers size={16} />, keywords: "add queue new", run: () => router.push("/queues?create=1") },
    ];

    const queueActions: CommandItem[] = queues.flatMap((q) => [
      {
        id: `pause-${q.id}`,
        label: q.is_paused ? `Resume queue ${q.name}` : `Pause queue ${q.name}`,
        section: "Queues",
        icon: q.is_paused ? <Play size={16} /> : <Pause size={16} />,
        keywords: `${q.name} ${q.slug} pause resume`,
        run: async () => {
          if (q.is_paused) await api.resumeQueue(q.id);
          else await api.pauseQueue(q.id);
          mutate("queues");
        },
      },
    ]);

    const dlqActions: CommandItem[] = dlq.slice(0, 8).map((d) => {
      const name = jobDisplayName({ payload: d.payload });
      return {
        id: `retry-${d.job_id}`,
        label: `Retry ${name} (${d.queue_name})`,
        section: "Dead letter",
        icon: <RotateCcw size={16} />,
        keywords: `${name} ${d.queue_name} retry dlq`,
        run: async () => {
          await api.retryJob(d.job_id);
          mutate("dlq");
          router.push("/dlq");
        },
      };
    });

    return [...nav, ...queueActions, ...dlqActions];
  }, [queues, dlq, router]);

  const filtered = useMemo(() => {
    return items
      .map((item) => ({ item, s: score(query, item) }))
      .filter((x) => x.s > 0)
      .sort((a, b) => b.s - a.s)
      .map((x) => x.item);
  }, [items, query]);

  useEffect(() => {
    if (!commandOpen) {
      setQuery("");
      setActive(0);
    }
  }, [commandOpen]);

  useEffect(() => {
    setActive(0);
  }, [query]);

  if (!commandOpen) return null;

  async function run(item: CommandItem) {
    setCommandOpen(false);
    await item.run();
  }

  return (
    <div className="fixed inset-0 z-[100] flex items-start justify-center pt-[15vh] px-4" role="dialog" aria-modal aria-label="Command palette">
      <button
        type="button"
        className="absolute inset-0"
        style={{ background: "rgba(10, 13, 19, 0.75)" }}
        onClick={() => setCommandOpen(false)}
        aria-label="Close command palette"
      />
      <div
        className="relative w-full max-w-lg overflow-hidden"
        style={{
          background: "var(--bg-panel)",
          border: "1px solid var(--border-hairline)",
          borderRadius: "var(--r-lg)",
          boxShadow: "inset 0 1px 0 rgba(255,255,255,0.03)",
        }}
      >
        <div
          className="flex items-center gap-3 px-4 py-3"
          style={{ borderBottom: "1px solid var(--border-hairline)" }}
        >
          <Search size={16} style={{ color: "var(--text-tertiary)" }} />
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setActive((a) => Math.min(a + 1, filtered.length - 1));
              } else if (e.key === "ArrowUp") {
                e.preventDefault();
                setActive((a) => Math.max(a - 1, 0));
              } else if (e.key === "Enter" && filtered[active]) {
                e.preventDefault();
                run(filtered[active]);
              } else if (e.key === "Escape") {
                setCommandOpen(false);
              }
            }}
            placeholder="Search commands…"
            className="input"
            style={{ border: "none", background: "transparent", padding: 0 }}
          />
          <kbd
            className="eyebrow hidden sm:inline"
            style={{
              padding: "4px 8px",
              border: "1px solid var(--border-strong)",
              borderRadius: "var(--r-sm)",
            }}
          >
            ESC
          </kbd>
        </div>
        <ul className="max-h-72 overflow-y-auto py-2" role="listbox">
          {filtered.length === 0 ? (
            <li className="px-4 py-8 text-center body-sm" style={{ color: "var(--text-tertiary)" }}>
              No matching commands
            </li>
          ) : (
            filtered.map((item, i) => (
              <li key={item.id}>
                <button
                  type="button"
                  role="option"
                  aria-selected={i === active}
                  onClick={() => run(item)}
                  className="w-full flex items-center gap-3 px-4 py-2.5 text-left transition-colors"
                  style={{
                    background: i === active ? "var(--accent-dim)" : "transparent",
                    color: "var(--text-primary)",
                  }}
                >
                  <span style={{ color: "var(--text-tertiary)" }}>{item.icon}</span>
                  <span className="flex-1 body-sm">{item.label}</span>
                  <span className="eyebrow">{item.section}</span>
                </button>
              </li>
            ))
          )}
        </ul>
      </div>
    </div>
  );
}
