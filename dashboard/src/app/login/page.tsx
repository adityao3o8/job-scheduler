"use client";

import { useState } from "react";
import { Activity } from "lucide-react";
import { api, setToken } from "@/lib/api";
import { Button } from "@/components/ui/Button";

export default function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      const { token } = await api.login(email.trim(), password);
      setToken(token);
      window.location.href = "/";
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Sign-in failed. Check API availability."
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <div
      className="min-h-screen flex items-center justify-center p-6"
      style={{ background: "var(--bg-void)" }}
    >
      <div className="w-full max-w-md">
        <div className="flex items-center gap-3 mb-8 justify-center">
          <div
            className="flex items-center justify-center"
            style={{
              width: 40,
              height: 40,
              background: "var(--accent-dim)",
              border: "1px solid var(--accent)",
              borderRadius: "var(--r-md)",
            }}
          >
            <Activity size={20} style={{ color: "var(--accent)" }} />
          </div>
          <div>
            <p className="eyebrow" style={{ color: "var(--accent)" }}>
              Scheduler
            </p>
            <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
              Operator console
            </p>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="panel p-6 space-y-4">
          <div>
            <h1 className="h1 mb-1">Enter console</h1>
            <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
              Demo mode — use any email and password to access the dashboard.
            </p>
          </div>

          {error && (
            <div
              className="p-3 body-sm"
              style={{
                background: "var(--st-failed-dim)",
                border: "1px solid var(--st-failed)",
                borderRadius: "var(--r-md)",
                color: "var(--st-failed)",
              }}
            >
              {error}
            </div>
          )}

          <label className="block">
            <span className="eyebrow mb-2 block">Email</span>
            <input
              type="text"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              className="input"
              autoComplete="username"
              placeholder="you@company.com"
            />
          </label>

          <label className="block">
            <span className="eyebrow mb-2 block">Password</span>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              className="input"
              autoComplete="current-password"
              placeholder="Any value works"
            />
          </label>

          <Button type="submit" disabled={loading} className="w-full">
            {loading ? "Opening console…" : "Open console"}
          </Button>
        </form>
      </div>
    </div>
  );
}
