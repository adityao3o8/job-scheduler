import { Pool, neon } from "@neondatabase/serverless";

function getUrl() {
  const url = process.env.DATABASE_URL || process.env.POSTGRES_URL;
  if (!url) {
    throw new Error("DATABASE_URL is not configured. Add Vercel Postgres (Neon) to the project.");
  }
  return url;
}

export function getSql() {
  return neon(getUrl());
}

export function getPool() {
  return new Pool({ connectionString: getUrl() });
}
