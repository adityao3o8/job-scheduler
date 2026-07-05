"use client";

import { useEffect, useState } from "react";
import { mutate } from "swr";
import { api } from "@/lib/api";
import { slugifyName } from "@/lib/format";
import type { Project } from "@/lib/types";
import { Button } from "@/components/ui/Button";

export function CreateQueueForm({
  projects,
  onCreated,
  onCancel,
}: {
  projects: Project[];
  onCreated: () => void;
  onCancel: () => void;
}) {
  const [projectId, setProjectId] = useState(projects[0]?.id ?? "");
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const [concurrency, setConcurrency] = useState("10");
  const [priority, setPriority] = useState("5");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!slugTouched && name) setSlug(slugifyName(name));
  }, [name, slugTouched]);

  useEffect(() => {
    if (!projectId && projects[0]) setProjectId(projects[0].id);
  }, [projects, projectId]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError("");
    try {
      await api.createQueue({
        project_id: projectId,
        name: name.trim(),
        slug: slug.trim(),
        priority_default: parseInt(priority, 10) || 5,
        concurrency_limit: concurrency ? parseInt(concurrency, 10) : undefined,
      });
      mutate("queues");
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create queue");
    } finally {
      setSaving(false);
    }
  }

  if (!projects.length) {
    return (
      <div className="space-y-4">
        <p className="body-sm" style={{ color: "var(--text-tertiary)" }}>
          No projects exist yet. Create a project first, then add queues under it.
        </p>
        <CreateProjectInline onCreated={() => mutate("projects")} />
        <Button variant="secondary" onClick={onCancel}>
          Cancel
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
        <span className="eyebrow mb-2 block">Project</span>
        <select className="input" value={projectId} onChange={(e) => setProjectId(e.target.value)} required>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      </label>

      <label className="block">
        <span className="eyebrow mb-2 block">Queue name</span>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} required placeholder="Emails" />
      </label>

      <label className="block">
        <span className="eyebrow mb-2 block">Slug</span>
        <input
          className="input mono-data"
          value={slug}
          onChange={(e) => {
            setSlugTouched(true);
            setSlug(e.target.value);
          }}
          required
          pattern="[a-z0-9]+(?:-[a-z0-9]+)*"
          placeholder="emails"
        />
      </label>

      <div className="grid grid-cols-2 gap-4">
        <label className="block">
          <span className="eyebrow mb-2 block">Concurrency limit</span>
          <input className="input mono-data" type="number" min={1} value={concurrency} onChange={(e) => setConcurrency(e.target.value)} />
        </label>
        <label className="block">
          <span className="eyebrow mb-2 block">Default priority</span>
          <input className="input mono-data" type="number" value={priority} onChange={(e) => setPriority(e.target.value)} />
        </label>
      </div>

      <div className="flex gap-2 pt-2">
        <Button type="submit" disabled={saving}>
          {saving ? "Creating…" : "Create queue"}
        </Button>
        <Button type="button" variant="secondary" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

function CreateProjectInline({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!slugTouched && name) setSlug(slugifyName(name));
  }, [name, slugTouched]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError("");
    try {
      await api.createProject({ name: name.trim(), slug: slug.trim() });
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create project");
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="panel p-4 space-y-3">
      <p className="eyebrow">New project</p>
      {error && <p className="body-sm" style={{ color: "var(--st-failed)" }}>{error}</p>}
      <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Project name" required />
      <input
        className="input mono-data"
        value={slug}
        onChange={(e) => {
          setSlugTouched(true);
          setSlug(e.target.value);
        }}
        placeholder="project-slug"
        required
      />
      <Button type="submit" size="sm" disabled={saving}>
        {saving ? "Creating…" : "Create project"}
      </Button>
    </form>
  );
}
