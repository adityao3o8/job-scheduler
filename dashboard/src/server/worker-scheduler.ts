import { runWorkerTick } from "./worker";

let inFlight: Promise<{ processed: number; worker: string }> | null = null;
let lastRunAt = 0;
const MIN_INTERVAL_MS = 2500;

/** Run at most one worker tick per interval; coalesce concurrent callers. */
export function scheduleWorkerTick(): void {
  const now = Date.now();
  if (inFlight || now - lastRunAt < MIN_INTERVAL_MS) return;
  lastRunAt = now;
  inFlight = runWorkerTick()
    .catch((err) => {
      console.error("worker tick failed:", err);
      return { processed: 0, worker: "vercel-worker-1" };
    })
    .finally(() => {
      inFlight = null;
    });
}

export async function runWorkerTickNow() {
  lastRunAt = Date.now();
  return runWorkerTick();
}
