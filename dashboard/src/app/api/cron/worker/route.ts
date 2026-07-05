import { runWorkerTick } from "@/server/worker";
import { json, error } from "@/server/http";

export const maxDuration = 60;

export async function GET(req: Request) {
  const secret = process.env.CRON_SECRET;
  if (secret) {
    const auth = req.headers.get("authorization");
    if (auth !== `Bearer ${secret}`) {
      return error("forbidden", 403, "FORBIDDEN");
    }
  }

  try {
    const result = await runWorkerTick();
    return json({ ok: true, ...result });
  } catch (e) {
    const msg = e instanceof Error ? e.message : "worker tick failed";
    return error(msg, 500);
  }
}
