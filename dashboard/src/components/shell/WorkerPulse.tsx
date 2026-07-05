"use client";

import { useWorkerPulse } from "@/lib/use-api";

/** Keeps the serverless worker ticking while the console is open. */
export function WorkerPulse() {
  useWorkerPulse();
  return null;
}
