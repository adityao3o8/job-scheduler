export interface Queue {
  id: string;
  project_id: string;
  name: string;
  slug: string;
  retry_policy_id?: string;
  priority_default: number;
  concurrency_limit?: number;
  is_paused: boolean;
  created_at: string;
  updated_at: string;
}

export interface QueueStats {
  queue_id: string;
  queue_name: string;
  depth: number;
  throughput_1h: number;
  throughput_24h: number;
  success_rate: number;
  avg_latency_ms: number;
  p95_latency_ms: number;
}

export interface Job {
  id: string;
  queue_id: string;
  status: string;
  priority: number;
  payload: Record<string, unknown>;
  idempotency_key?: string;
  next_run_at?: string;
  attempts: number;
  max_attempts: number;
  retry_policy_id?: string;
  worker_id?: string;
  claimed_at?: string;
  lease_expires_at?: string;
  completed_at?: string;
  failed_at?: string;
  error_message?: string;
  cron_expr?: string;
  batch_id?: string;
  created_at: string;
  updated_at: string;
}

export interface JobExecution {
  id: string;
  job_id: string;
  worker_id: string;
  attempt: number;
  status: string;
  started_at: string;
  finished_at?: string;
  error?: string;
  duration_ms?: number;
}

export interface JobLog {
  id: string;
  job_id: string;
  level: string;
  message: string;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface WorkerInfo {
  id: string;
  name: string;
  status: string;
  last_seen_at?: string;
  jobs_in_flight: number;
  created_at: string;
  updated_at: string;
}

export interface DLQItem {
  id: string;
  job_id: string;
  original_queue_id: string;
  queue_name: string;
  payload: Record<string, unknown>;
  failed_at: string;
  reason?: string;
  attempts_made: number;
  created_at: string;
}

export interface Page<T> {
  items: T[];
  next_cursor?: string;
  has_more: boolean;
}

export interface Project {
  id: string;
  org_id: string;
  name: string;
  slug: string;
  description?: string;
  created_at: string;
  updated_at: string;
}
