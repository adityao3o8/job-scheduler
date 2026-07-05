"use client";

import { useCallback, useEffect, useRef } from "react";

export type PulseEventType = "completed" | "failed";

export interface PulseEvent {
  type: PulseEventType;
  at: number;
}

interface PulseProps {
  events: PulseEvent[];
  compact?: boolean;
  className?: string;
}

const COMPLETED = "#43C98A";
const FAILED = "#F26759";
const BASELINE = "#232B38";
const SCAN = "rgba(76, 157, 245, 0.15)";

export function Pulse({ events, compact = false, className = "" }: PulseProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const frameRef = useRef<number>(0);
  const pointsRef = useRef<{ y: number; color?: string }[]>([]);
  const scanRef = useRef(0);
  const eventsRef = useRef(events);

  useEffect(() => {
    eventsRef.current = events;
  }, [events]);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const dpr = window.devicePixelRatio || 1;
    const w = canvas.clientWidth;
    const h = canvas.clientHeight;
    if (canvas.width !== w * dpr || canvas.height !== h * dpr) {
      canvas.width = w * dpr;
      canvas.height = h * dpr;
      ctx.scale(dpr, dpr);
    }

    const mid = h / 2;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    // Ingest new events as spikes
    const now = Date.now();
    const recent = eventsRef.current.filter((e) => now - e.at < 3000);
    if (recent.length > 0 && !reduced) {
      const last = recent[recent.length - 1];
      const amp = compact ? 0.35 : 0.55;
      const y =
        last.type === "completed"
          ? mid - h * amp
          : mid + h * amp;
      pointsRef.current.push({
        y,
        color: last.type === "completed" ? COMPLETED : FAILED,
      });
    }

    // Baseline wander + decay
    const lastY = pointsRef.current.length
      ? pointsRef.current[pointsRef.current.length - 1].y
      : mid;
    const wander = mid + (Math.sin(now / 400) * (compact ? 1.5 : 2.5));
    const nextY = lastY * 0.82 + wander * 0.18;
    pointsRef.current.push({ y: nextY });

    const maxPts = compact ? 80 : 160;
    if (pointsRef.current.length > maxPts) {
      pointsRef.current = pointsRef.current.slice(-maxPts);
    }

    ctx.clearRect(0, 0, w, h);

    // Grid
    ctx.strokeStyle = BASELINE;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, mid);
    ctx.lineTo(w, mid);
    ctx.stroke();

    // Scan line
    if (!reduced) {
      scanRef.current = (scanRef.current + (compact ? 1.2 : 2)) % w;
      const grad = ctx.createLinearGradient(scanRef.current - 40, 0, scanRef.current + 40, 0);
      grad.addColorStop(0, "transparent");
      grad.addColorStop(0.5, SCAN);
      grad.addColorStop(1, "transparent");
      ctx.fillStyle = grad;
      ctx.fillRect(scanRef.current - 40, 0, 80, h);
    }

    // Trace
    const pts = pointsRef.current;
    const step = w / Math.max(pts.length - 1, 1);
    ctx.lineWidth = compact ? 1.25 : 1.75;
    ctx.lineJoin = "round";
    ctx.lineCap = "round";

    for (let i = 1; i < pts.length; i++) {
      const p0 = pts[i - 1];
      const p1 = pts[i];
      ctx.strokeStyle = p1.color ?? "#4C9DF5";
      ctx.globalAlpha = p1.color ? 0.95 : 0.55;
      ctx.beginPath();
      ctx.moveTo((i - 1) * step, p0.y);
      ctx.lineTo(i * step, p1.y);
      ctx.stroke();
    }
    ctx.globalAlpha = 1;

    if (!reduced) {
      frameRef.current = requestAnimationFrame(draw);
    }
  }, [compact]);

  useEffect(() => {
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (reduced) {
      draw();
      return;
    }
    frameRef.current = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(frameRef.current);
  }, [draw]);

  const height = compact ? 32 : 72;

  return (
    <canvas
      ref={canvasRef}
      className={className}
      style={{
        width: compact ? 120 : "100%",
        height,
        display: "block",
        background: "var(--bg-void)",
        borderRadius: compact ? "var(--r-sm)" : "var(--r-md)",
        border: "1px solid var(--border-hairline)",
      }}
      aria-label="System pulse — live job activity"
    />
  );
}
