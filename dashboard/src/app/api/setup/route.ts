import { ensureSchema, seedDemo } from "@/server/setup";
import { json, error } from "@/server/http";

export const maxDuration = 60;

export async function POST(req: Request) {
  const secret = process.env.SETUP_SECRET || process.env.CRON_SECRET;
  const auth = req.headers.get("authorization");
  if (secret && auth !== `Bearer ${secret}`) {
    return error("forbidden", 403, "FORBIDDEN");
  }

  try {
    await ensureSchema();
    const seed = await seedDemo();
    return json({ ok: true, schema: true, ...seed });
  } catch (e) {
    const msg = e instanceof Error ? e.message : "setup failed";
    return error(msg, 500);
  }
}

export async function GET() {
  return json({ endpoint: "POST /api/setup to initialize database and seed demo data" });
}
