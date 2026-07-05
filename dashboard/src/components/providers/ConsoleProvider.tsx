"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useJobs } from "@/lib/use-api";
import type { PulseEvent } from "@/components/ui/Pulse";

interface ConsoleContextValue {
  pulseEvents: PulseEvent[];
  commandOpen: boolean;
  setCommandOpen: (open: boolean) => void;
  openCommand: () => void;
  submitJobRequest: number;
  createQueueRequest: number;
  openSubmitJob: () => void;
  openCreateQueue: () => void;
}

const ConsoleContext = createContext<ConsoleContextValue | null>(null);

export function ConsoleProvider({ children }: { children: React.ReactNode }) {
  const { data: jobPage } = useJobs();
  const [pulseEvents, setPulseEvents] = useState<PulseEvent[]>([]);
  const [commandOpen, setCommandOpen] = useState(false);
  const [submitJobRequest, setSubmitJobRequest] = useState(0);
  const [createQueueRequest, setCreateQueueRequest] = useState(0);
  const prevRef = useRef({ completed: 0, failed: 0, dead: 0 });

  useEffect(() => {
    if (!jobPage?.items) return;
    const completed = jobPage.items.filter((j) => j.status === "completed").length;
    const failed = jobPage.items.filter((j) => j.status === "failed").length;
    const dead = jobPage.items.filter((j) => j.status === "dead_letter").length;
    const prev = prevRef.current;
    const now = Date.now();
    const next: PulseEvent[] = [];

    if (completed > prev.completed) {
      for (let i = 0; i < completed - prev.completed; i++) {
        next.push({ type: "completed", at: now + i });
      }
    }
    const failTotal = failed + dead;
    const prevFail = prev.failed + prev.dead;
    if (failTotal > prevFail) {
      for (let i = 0; i < failTotal - prevFail; i++) {
        next.push({ type: "failed", at: now + i });
      }
    }

    if (next.length) {
      setPulseEvents((e) => [...e, ...next].slice(-50));
    }
    prevRef.current = { completed, failed, dead };
  }, [jobPage]);

  const openCommand = useCallback(() => setCommandOpen(true), []);
  const openSubmitJob = useCallback(() => setSubmitJobRequest((n) => n + 1), []);
  const openCreateQueue = useCallback(() => setCreateQueueRequest((n) => n + 1), []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCommandOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const value = useMemo(
    () => ({
      pulseEvents,
      commandOpen,
      setCommandOpen,
      openCommand,
      submitJobRequest,
      createQueueRequest,
      openSubmitJob,
      openCreateQueue,
    }),
    [
      pulseEvents,
      commandOpen,
      openCommand,
      submitJobRequest,
      createQueueRequest,
      openSubmitJob,
      openCreateQueue,
    ]
  );

  return <ConsoleContext.Provider value={value}>{children}</ConsoleContext.Provider>;
}

export function useConsole() {
  const ctx = useContext(ConsoleContext);
  if (!ctx) throw new Error("useConsole must be used within ConsoleProvider");
  return ctx;
}
