"use client";

import { useEffect } from "react";
import { X } from "lucide-react";

interface DrawerProps {
  open: boolean;
  onClose: () => void;
  title: string;
  subtitle?: string;
  children: React.ReactNode;
  width?: number;
  actions?: React.ReactNode;
}

export function Drawer({
  open,
  onClose,
  title,
  subtitle,
  children,
  width = 480,
  actions,
}: DrawerProps) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = "";
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex justify-end" role="dialog" aria-modal>
      <button
        type="button"
        className="absolute inset-0"
        style={{ background: "rgba(10, 13, 19, 0.72)" }}
        onClick={onClose}
        aria-label="Close drawer"
      />
      <aside
        className="relative flex flex-col h-full"
        style={{
          width: "min(100vw, " + width + "px)",
          background: "var(--bg-panel)",
          borderLeft: "1px solid var(--border-hairline)",
          boxShadow: "inset 0 1px 0 rgba(255,255,255,0.03)",
        }}
      >
        <header
          className="flex items-start justify-between gap-4 px-6 py-4 shrink-0"
          style={{ borderBottom: "1px solid var(--border-hairline)" }}
        >
          <div>
            <h2 className="h2">{title}</h2>
            {subtitle && (
              <p className="mono-data body-sm mt-1" style={{ color: "var(--text-tertiary)" }}>
                {subtitle}
              </p>
            )}
          </div>
          <div className="flex items-center gap-2">
            {actions}
            <button
              type="button"
              onClick={onClose}
              className="btn btn-ghost p-2"
              aria-label="Close"
            >
              <X size={16} />
            </button>
          </div>
        </header>
        <div className="flex-1 overflow-y-auto px-6 py-4">{children}</div>
      </aside>
    </div>
  );
}
