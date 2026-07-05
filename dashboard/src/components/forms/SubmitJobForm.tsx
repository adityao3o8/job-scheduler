"use client";

import { useEffect, useState } from "react";
import { mutate } from "swr";
import { api } from "@/lib/api";
import type { Queue } from "@/lib/types";
import { Button } from "@/components/ui/Button";

const JOB_TEMPLATES = {
  sleep: { type: "sleep", duration_ms: 500 },
  always_fail: { type: "always_fail", message: "simulated failure" },
  webhook: { type: "webhook", url: "https://example.com/hook", method: "POST" },
} as const;

type JobTemplate = keyof typeof JOB_TEMPLATES | "custom";

export function SubmitJobForm({
  queues,
  defaultQueueId,
  onSubmitted,
  onCancel,
}: {
  queues: Queue[];
  defaultQueueId?: string;
  onSubmitted: (jobId: string) => void;
  onCancel: () => void;
}) {
  const [queueId, setQueueId] = useState(defaultQueueId ?? queues[0]?.id ?? "");
  const [jobName, setJobName] = useState("");
  const [template, setTemplate] = useState<JobTemplate>("sleep");
  const [durationMs, setDurationMs] = useState("500");
  const [failMessage, setFailMessage] = useState("simulated failure");
  const [priority, setPriority] = useState("");
  const [maxAttempts, setMaxAttempts] = useState("3");
  const [delaySeconds, setDelaySeconds] = useState("");
  const [cronExpr, setCronExpr] = useState("");
  const [customPayload, setCustomPayload] = useState(
    JSON.stringify(JOB_TEMPLATES.sleep, null, 2)
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!queueId && queues[0]) setQueueId(queues[0].id);
  }, [queues, queueId]);

  useEffect(() => {
    if (template === "custom") return;
    if (template === "sleep") {
      setCustomPayload(JSON.stringify({ type: "sleep", duration_ms: parseInt(durationMs, 10) || 500 }, null, 2));
    } else if (template === "always_fail") {
      setCustomPayload(JSON.stringify({ type: "always_fail", message: failMessage }, null, 2));
    } else {
      setCustomPayload(JSON.stringify(JOB_TEMPLATES[template], null, 2));
    }
  }, [template, durationMs, failMessage]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError("");
    try {
      let payload: Record<string, unknown>;
      try {
        payload = JSON.parse(customPayload) as Record<string, unknown>;
      } catch {
        setError("Payload must be valid JSON.");
        setSaving(false);
        return;
      }

      const job = await api.enqueueJob(queueId, {
        payload,
        ...(jobName.trim() ? { idempotency_key: jobName.trim() } : {}),
        ...(priority ? { priority: parseInt(priority, 10) } : {}),
        ...(maxAttempts ? { max_attempts: parseInt(maxAttempts, 10) } : {}),
        ...(delaySeconds ? { delay_seconds: parseInt(delaySeconds, 10) } : {}),
        ...(cronExpr.trim() ? { cron_expr: cronExpr.trim() } : {}),
      });
      mutate((key) => typeof key === "string" && key.startsWith("jobs"));
      onSubmitted(job.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to submit job");
    } finally {
      setSaving(false);
    }
  }

  if (!queues.length) {
    return (
      <div className="space-y-4">
        <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
          Create a queue first, then submit work to it.
        </p>
        <Button variant="secondary" onClick={onCancel}>
          Close
        </Button>
      </div>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && (
        <p className="body-sm panel p-3" style={{ color: "var(--st-failed)", borderColor: "var(--st-failed)" }}>
          {error}
        </p>
      )}

      <label className="block">
        <span className="eyebrow mb-2 block">Queue</span>
        <select className="input" value={queueId} onChange={(e) => setQueueId(e.target.value)} required>
          {queues.map((q) => (
            <option key={q.id} value={q.id}>
              {q.name}
            </option>
          ))}
        </select>
      </label>

      <label className="block">
        <span className="eyebrow mb-2 block">Job name</span>
        <input
          className="input"
          value={jobName}
          onChange={(e) => setJobName(e.target.value)}
          placeholder="payment-abc-123 (idempotency key)"
        />
        <span className="body-sm mt-1 block" style={{ color: "var(--text-tertiary)" }}>
          Optional. Shown in the explorer and prevents duplicate runs.
        </span>
      </label>

      <label className="block">
        <span className="eyebrow mb-2 block">Job type</span>
        <select className="input" value={template} onChange={(e) => setTemplate(e.target.value as JobTemplate)}>
          <option value="sleep">Sleep (demo)</option>
          <option value="webhook">Webhook</option>
          <option value="always_fail">Always fail (test DLQ)</option>
          <option value="custom">Custom JSON</option>
        </select>
      </label>

      {template === "sleep" && (
        <label className="block">
          <span className="eyebrow mb-2 block">Duration (ms)</span>
          <input className="input mono-data" type="number" min={1} value={durationMs} onChange={(e) => setDurationMs(e.target.value)} />
        </label>
      )}

      {template === "always_fail" && (
        <label className="block">
          <span className="eyebrow mb-2 block">Error message</span>
          <input className="input" value={failMessage} onChange={(e) => setFailMessage(e.target.value)} />
        </label>
      )}

      {(template === "custom" || template === "webhook") && (
        <label className="block">
          <span className="eyebrow mb-2 block">Payload</span>
          <textarea
            className="input mono-data min-h-[120px]"
            value={customPayload}
            onChange={(e) => setCustomPayload(e.target.value)}
            spellCheck={false}
          />
        </label>
      )}

      <details>
        <summary className="eyebrow cursor-pointer select-none">Scheduling & retries</summary>
        <div className="grid grid-cols-2 gap-4 mt-4">
          <label className="block">
            <span className="eyebrow mb-2 block">Priority</span>
            <input className="input mono-data" type="number" value={priority} onChange={(e) => setPriority(e.target.value)} placeholder="Queue default" />
          </label>
          <label className="block">
            <span className="eyebrow mb-2 block">Max attempts</span>
            <input className="input mono-data" type="number" min={1} value={maxAttempts} onChange={(e) => setMaxAttempts(e.target.value)} />
          </label>
          <label className="block">
            <span className="eyebrow mb-2 block">Delay (seconds)</span>
            <input className="input mono-data" type="number" min={0} value={delaySeconds} onChange={(e) => setDelaySeconds(e.target.value)} placeholder="Run immediately" />
          </label>
          <label className="block col-span-2">
            <span className="eyebrow mb-2 block">Cron expression</span>
            <input className="input mono-data" value={cronExpr} onChange={(e) => setCronExpr(e.target.value)} placeholder="*/5 * * * *" />
          </label>
        </div>
      </details>

      <div className="flex gap-2 pt-2">
        <Button type="submit" disabled={saving}>
          {saving ? "Submitting…" : "Submit job"}
        </Button>
        <Button type="button" variant="secondary" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
