const BASE = "/api";

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("scheduler_token");
}

export function setToken(token: string) {
  localStorage.setItem("scheduler_token", token);
}

export function clearToken() {
  localStorage.removeItem("scheduler_token");
}

export function hasToken(): boolean {
  return !!getToken();
}

type RequestOpts = RequestInit & { auth?: boolean };

async function request<T>(path: string, init?: RequestOpts): Promise<T> {
  const { auth = true, ...fetchInit } = init ?? {};
  const token = getToken();
  let res: Response;
  try {
    res = await fetch(`${BASE}${path}`, {
      ...fetchInit,
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...fetchInit.headers,
      },
    });
  } catch {
    throw new Error(
      "Cannot reach the API. Check your connection or redeploy the Vercel app."
    );
  }
  if (res.status === 401 && auth) {
    clearToken();
    if (typeof window !== "undefined" && window.location.pathname !== "/login") {
      window.location.href = "/login";
    }
    throw new Error("Session expired. Sign in again.");
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  return res.json();
}

export const api = {
  // Auth — no session redirect on 401 so wrong password shows inline
  login: (email: string, password: string) =>
    request<{ token: string }>("/auth/login", {
      method: "POST",
      auth: false,
      body: JSON.stringify({ email, password }),
    }),

  // Projects
  listProjects: () => request<Page<Project>>("/projects"),
  createProject: (data: { name: string; slug: string; description?: string }) =>
    request<Project>("/projects", { method: "POST", body: JSON.stringify(data) }),

  // Queues
  listQueues: () => request<Page<Queue>>("/queues"),
  createQueue: (data: {
    project_id: string;
    name: string;
    slug: string;
    priority_default?: number;
    concurrency_limit?: number;
  }) => request<Queue>("/queues", { method: "POST", body: JSON.stringify(data) }),
  getQueue: (id: string) => request<Queue>(`/queues/${id}`),
  getQueueStats: (id: string) => request<QueueStats>(`/queues/${id}/stats`),
  updateQueue: (id: string, data: Partial<Queue>) =>
    request<Queue>(`/queues/${id}`, { method: "PUT", body: JSON.stringify(data) }),
  pauseQueue: (id: string) =>
    request<void>(`/queues/${id}/pause`, { method: "POST" }),
  resumeQueue: (id: string) =>
    request<void>(`/queues/${id}/resume`, { method: "POST" }),

  // Jobs
  listJobs: (params?: Record<string, string>) => {
    const qs = new URLSearchParams(params).toString();
    return request<Page<Job>>(`/jobs${qs ? `?${qs}` : ""}`);
  },
  listQueueJobs: (queueId: string, params?: Record<string, string>) => {
    const qs = new URLSearchParams(params).toString();
    return request<Page<Job>>(`/queues/${queueId}/jobs${qs ? `?${qs}` : ""}`);
  },
  getJob: (id: string) => request<Job>(`/jobs/${id}`),
  getJobExecutions: (id: string) => request<JobExecution[]>(`/jobs/${id}/executions`),
  getJobLogs: (id: string) => request<JobLog[]>(`/jobs/${id}/logs`),
  retryJob: (id: string) =>
    request<Job>(`/jobs/${id}/retry`, { method: "POST" }),
  enqueueJob: (
    queueId: string,
    data: {
      payload: Record<string, unknown>;
      priority?: number;
      max_attempts?: number;
      idempotency_key?: string;
      delay_seconds?: number;
      cron_expr?: string;
    }
  ) =>
    request<Job>(`/queues/${queueId}/jobs`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  // Workers
  listWorkers: () => request<WorkerInfo[]>("/workers"),
  workerPulse: () => request<{ ok: boolean; processed: number; worker: string }>("/worker/pulse"),

  // DLQ
  listDLQ: () => request<Page<DLQItem>>("/dlq"),
};

import type { Queue, QueueStats, Job, JobExecution, JobLog, WorkerInfo, DLQItem, Page, Project } from "./types";
