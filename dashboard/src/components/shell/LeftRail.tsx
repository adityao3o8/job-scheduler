"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Activity,
  Layers,
  ListTodo,
  Server,
  Skull,
  LogOut,
} from "lucide-react";
import { clearToken } from "@/lib/api";

const NAV = [
  { href: "/", label: "Overview", icon: Activity },
  { href: "/queues", label: "Queues", icon: Layers },
  { href: "/jobs", label: "Jobs", icon: ListTodo },
  { href: "/workers", label: "Workers", icon: Server },
  { href: "/dlq", label: "Dead Letter", icon: Skull },
];

export function LeftRail() {
  const path = usePathname();

  return (
    <>
      <aside
        className="hidden md:flex flex-col shrink-0 h-full"
        style={{
          width: "var(--rail-width)",
          background: "var(--bg-panel)",
          borderRight: "1px solid var(--border-hairline)",
        }}
      >
        <div className="px-4 py-5" style={{ borderBottom: "1px solid var(--border-hairline)" }}>
          <Link href="/" className="flex items-center gap-3 no-underline">
            <div
              className="flex items-center justify-center shrink-0"
              style={{
                width: 32,
                height: 32,
                background: "var(--accent-dim)",
                border: "1px solid var(--accent)",
                borderRadius: "var(--r-sm)",
              }}
            >
              <Activity size={16} style={{ color: "var(--accent)" }} />
            </div>
            <div className="rail-label">
              <span className="eyebrow" style={{ color: "var(--accent)" }}>
                Scheduler
              </span>
              <span className="body-sm block" style={{ color: "var(--text-tertiary)" }}>
                Control plane
              </span>
            </div>
          </Link>
        </div>

        <nav className="flex-1 py-3 px-2 space-y-1">
          {NAV.map(({ href, label, icon: Icon }) => {
            const active = href === "/" ? path === "/" : path.startsWith(href);
            return (
              <Link
                key={href}
                href={href}
                className="flex items-center gap-3 px-3 py-2.5 rounded-md transition-colors no-underline rail-link"
                style={{
                  background: active ? "var(--accent-dim)" : "transparent",
                  borderLeft: active ? "2px solid var(--accent)" : "2px solid transparent",
                  color: active ? "var(--text-primary)" : "var(--text-secondary)",
                }}
              >
                <Icon size={18} strokeWidth={1.75} />
                <span className="rail-label body-sm">{label}</span>
              </Link>
            );
          })}
        </nav>

        <div className="p-3" style={{ borderTop: "1px solid var(--border-hairline)" }}>
          <button
            type="button"
            onClick={() => {
              clearToken();
              window.location.href = "/login";
            }}
            className="btn btn-ghost w-full justify-start gap-3 rail-link"
          >
            <LogOut size={16} />
            <span className="rail-label">Sign out</span>
          </button>
        </div>
      </aside>

      {/* Mobile bottom nav */}
      <nav
        className="md:hidden fixed bottom-0 left-0 right-0 z-40 flex justify-around px-2 py-2"
        style={{
          background: "var(--bg-panel)",
          borderTop: "1px solid var(--border-hairline)",
        }}
      >
        {NAV.map(({ href, label, icon: Icon }) => {
          const active = href === "/" ? path === "/" : path.startsWith(href);
          return (
            <Link
              key={href}
              href={href}
              className="flex flex-col items-center gap-1 px-2 py-1 no-underline"
              style={{ color: active ? "var(--accent)" : "var(--text-tertiary)" }}
              aria-label={label}
            >
              <Icon size={20} strokeWidth={1.75} />
              <span className="eyebrow" style={{ fontSize: 9 }}>
                {label.split(" ")[0]}
              </span>
            </Link>
          );
        })}
      </nav>
    </>
  );
}
