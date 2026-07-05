"use client";

import { useEffect, useState } from "react";
import { Check } from "lucide-react";

export function CopyId({ id, display }: { id: string; display?: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    await navigator.clipboard.writeText(id);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        copy();
      }}
      className="mono-data body-sm inline-flex items-center gap-1.5 hover:opacity-80 transition-opacity"
      style={{ color: "var(--accent)" }}
      title="Copy full ID"
    >
      {display ?? id}
      {copied ? <Check size={12} /> : null}
    </button>
  );
}

export function RelativeTime({ ts }: { ts: string }) {
  const [label, setLabel] = useState("");

  useEffect(() => {
    function tick() {
      const diff = Date.now() - new Date(ts).getTime();
      const secs = Math.floor(diff / 1000);
      if (secs < 5) setLabel("just now");
      else if (secs < 60) setLabel(`${secs}s ago`);
      else if (secs < 3600) setLabel(`${Math.floor(secs / 60)}m ago`);
      else if (secs < 86400) setLabel(`${Math.floor(secs / 3600)}h ago`);
      else setLabel(`${Math.floor(secs / 86400)}d ago`);
    }
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [ts]);

  return (
    <time dateTime={ts} className="mono-data body-sm" style={{ color: "var(--text-tertiary)" }}>
      {label || "—"}
    </time>
  );
}
