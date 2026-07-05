"use client";

import { getStatusStyle } from "@/lib/status";

interface StatusChipProps {
  status: string;
  pulse?: boolean;
  className?: string;
}

export function StatusChip({ status, pulse = false, className = "" }: StatusChipProps) {
  const style = getStatusStyle(status);
  const shouldPulse = pulse || status === "running" || status === "active";

  return (
    <span
      className={`status-chip ${className}`}
      style={{ background: style.dim, color: style.color }}
    >
      <span
        className={`status-dot ${shouldPulse ? "pulse" : ""}`}
        style={{ background: style.color }}
      />
      {style.label}
    </span>
  );
}
