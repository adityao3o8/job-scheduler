"use client";

import { Sparkline } from "./Sparkline";

interface StatPanelProps {
  label: string;
  value: string | number;
  delta?: string;
  deltaPositive?: boolean;
  sparkline?: number[];
  sparkColor?: string;
  loading?: boolean;
  className?: string;
}

export function StatPanel({
  label,
  value,
  delta,
  deltaPositive,
  sparkline,
  sparkColor,
  loading,
  className = "",
}: StatPanelProps) {
  if (loading) {
    return (
      <div className={`panel p-4 ${className}`}>
        <div className="skeleton h-3 w-20 mb-3" />
        <div className="skeleton h-8 w-24 mb-2" />
        <div className="skeleton h-4 w-16" />
      </div>
    );
  }

  return (
    <div className={`panel p-4 flex flex-col gap-2 ${className}`}>
      <span className="eyebrow">{label}</span>
      <div className="flex items-end justify-between gap-3">
        <span className="display-xl mono-data" style={{ color: "var(--text-primary)" }}>
          {value}
        </span>
        {sparkline && <Sparkline data={sparkline} color={sparkColor} />}
      </div>
      {delta && (
        <span
          className="body-sm mono-data"
          style={{
            color:
              deltaPositive === undefined
                ? "var(--text-tertiary)"
                : deltaPositive
                  ? "var(--st-completed)"
                  : "var(--st-failed)",
          }}
        >
          {delta}
        </span>
      )}
    </div>
  );
}
