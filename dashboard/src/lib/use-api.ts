"use client";
import useSWR, { type SWRConfiguration } from "swr";
import { api } from "./api";
import type { Queue, QueueStats, Job, JobExecution, JobLog, WorkerInfo, DLQItem, Page, Project } from "./types";

const POLL: SWRConfiguration = { refreshInterval: 3000 };
// NOTE: For production, replace SWR polling with WebSocket subscriptions.
// The server would push updates on job state changes, worker heartbeats,
// and queue depth changes via a /ws endpoint, feeding SWR's mutate().

export function useQueues() {
  return useSWR<Page<Queue>>("queues", () => api.listQueues(), POLL);
}

export function useProjects() {
  return useSWR<Page<Project>>("projects", () => api.listProjects(), POLL);
}

export function useQueueStats(id: string | undefined) {
  return useSWR<QueueStats>(
    id ? `queue-stats-${id}` : null,
    () => api.getQueueStats(id!),
    POLL
  );
}

export function useAllQueueStats(queueIds: string[]) {
  return useSWR<QueueStats[]>(
    queueIds.length ? `all-queue-stats-${queueIds.join(",")}` : null,
    () => Promise.all(queueIds.map((id) => api.getQueueStats(id))),
    POLL
  );
}

export function useJobs(params?: Record<string, string>) {
  const key = `jobs-${JSON.stringify(params || {})}`;
  return useSWR<Page<Job>>(key, () => api.listJobs(params), POLL);
}

export function useJob(id: string | undefined) {
  return useSWR<Job>(id ? `job-${id}` : null, () => api.getJob(id!), POLL);
}

export function useJobExecutions(id: string | undefined) {
  return useSWR<JobExecution[]>(
    id ? `job-exec-${id}` : null,
    () => api.getJobExecutions(id!),
    POLL
  );
}

export function useJobLogs(id: string | undefined) {
  return useSWR<JobLog[]>(
    id ? `job-logs-${id}` : null,
    () => api.getJobLogs(id!),
    POLL
  );
}

export function useWorkers() {
  return useSWR<WorkerInfo[]>("workers", () => api.listWorkers(), POLL);
}

export function useWorkerPulse() {
  return useSWR("worker-pulse", () => api.workerPulse(), POLL);
}

export function useDLQ() {
  return useSWR<Page<DLQItem>>("dlq", () => api.listDLQ(), POLL);
}
