const API_BASE = 'http://localhost:8080';

export interface Project {
  id: string;
  name: string;
  api_key: string;
  created_at: string;
}

export interface Queue {
  id: string;
  name: string;
  priority: number;
  concurrency_limit: number;
  is_paused: boolean;
  created_at: string;
  stats?: {
    queued_count: number;
    running_count: number;
    completed_count: number;
    failed_count: number;
  };
}

export interface Job {
  id: string;
  job_type: string;
  status: 'queued' | 'scheduled' | 'claimed' | 'running' | 'completed' | 'failed' | 'dead_letter' | 'cancelled';
  priority: number;
  run_at: string;
  attempt: number;
  created_at: string;
  updated_at: string;
}

export interface JobExecution {
  id: string;
  worker_id: string | null;
  attempt: number;
  started_at: string;
  finished_at: string | null;
  outcome: 'success' | 'failure' | 'timeout' | null;
  error_message: string | null;
  duration_ms: number | null;
}

export interface JobLog {
  level: 'info' | 'warn' | 'error';
  message: string;
  logged_at: string;
}

export interface JobDetails extends Job {
  queue_id: string;
  payload: Record<string, any>;
  claimed_by: string | null;
  lease_expires_at: string | null;
  executions: JobExecution[];
  logs: JobLog[];
  ai_summary?: string;
}

export interface WorkerInfo {
  id: string;
  hostname: string;
  started_at: string;
  last_seen_at: string;
  status: 'active' | 'draining' | 'dead';
  current_job_id?: string;
  current_job_type?: string;
}

export interface DLQEntry {
  id: string;
  original_job_id: string;
  queue_id: string;
  queue_name: string;
  payload: Record<string, any>;
  failure_reason: string;
  attempts_made: number;
  moved_at: string;
}

export interface QueueSize {
  queue_id: string;
  name: string;
  size: number;
}

export interface SystemHealth {
  queue_sizes: QueueSize[] | null;
  active_workers: number;
  failed_count: number;
  success_count: number;
  dlq_count: number;
  avg_duration_ms: number;
}

export interface ThroughputPoint {
  hour: string;
  successes: number;
  failures: number;
}

async function request(path: string, options: RequestInit = {}): Promise<any> {
  const token = localStorage.getItem('obsidian_token');
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  };

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      ...headers,
      ...options.headers,
    },
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || `HTTP error ${response.status}`);
  }

  return response.json();
}

export const api = {
  // Auth
  login: async (email: string, password: string): Promise<{ token: string; userId: string }> => {
    const res = await request('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });
    localStorage.setItem('obsidian_token', res.token);
    return res;
  },

  register: async (email: string, password: string): Promise<{ token: string; userId: string }> => {
    const res = await request('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });
    localStorage.setItem('obsidian_token', res.token);
    return res;
  },

  logout: () => {
    localStorage.removeItem('obsidian_token');
  },

  isLoggedIn: (): boolean => {
    return !!localStorage.getItem('obsidian_token');
  },

  // Projects
  createProject: (name: string): Promise<Project> => {
    return request('/api/projects', {
      method: 'POST',
      body: JSON.stringify({ name }),
    });
  },

  listProjects: (): Promise<Project[]> => {
    return request('/api/projects');
  },

  // Queues
  createQueue: (projectId: string, name: string, priority: number, concurrencyLimit: number): Promise<Queue> => {
    return request(`/api/projects/${projectId}/queues`, {
      method: 'POST',
      body: JSON.stringify({ name, priority, concurrency_limit: concurrencyLimit }),
    });
  },

  listQueues: (projectId: string): Promise<Queue[]> => {
    return request(`/api/projects/${projectId}/queues`);
  },

  updateQueue: (queueId: string, priority: number, concurrencyLimit: number, isPaused: boolean): Promise<Queue> => {
    return request(`/api/queues/${queueId}`, {
      method: 'PUT',
      body: JSON.stringify({ priority, concurrency_limit: concurrencyLimit, is_paused: isPaused }),
    });
  },

  // Jobs
  createJob: (
    queueId: string,
    jobType: string,
    payload: Record<string, any>,
    priority: number,
    runAt?: string,
    cronExpr?: string,
    dependsOn?: string[],
    batchId?: string
  ): Promise<any> => {
    return request(`/api/queues/${queueId}/jobs`, {
      method: 'POST',
      body: JSON.stringify({
        job_type: jobType,
        payload,
        priority,
        run_at: runAt,
        cron_expr: cronExpr,
        depends_on: dependsOn,
        batch_id: batchId,
      }),
    });
  },

  listJobs: (queueId: string, filters: { status?: string; job_type?: string } = {}): Promise<Job[]> => {
    const params = new URLSearchParams();
    if (filters.status) params.append('status', filters.status);
    if (filters.job_type) params.append('job_type', filters.job_type);
    const queryString = params.toString() ? `?${params.toString()}` : '';
    return request(`/api/queues/${queueId}/jobs${queryString}`).then((res: any) => {
      // Handle both paginated response {data: [...]} and raw array for backwards compat
      return Array.isArray(res) ? res : (res.data || []);
    });
  },

  getJob: (jobId: string): Promise<JobDetails> => {
    return request(`/api/jobs/${jobId}`);
  },

  retryJob: (jobId: string): Promise<{ message: string }> => {
    return request(`/api/jobs/${jobId}/retry`, { method: 'POST' });
  },

  cancelJob: (jobId: string): Promise<{ message: string }> => {
    return request(`/api/jobs/${jobId}/cancel`, { method: 'POST' });
  },

  // Workers
  listWorkers: (): Promise<WorkerInfo[]> => {
    return request('/api/workers');
  },

  // Dead Letter Queue
  listDLQ: (): Promise<DLQEntry[]> => {
    return request('/api/dlq');
  },

  // Metrics
  getSystemHealth: (): Promise<SystemHealth> => {
    return request('/api/metrics/system-health');
  },

  getThroughput: (): Promise<ThroughputPoint[]> => {
    return request('/api/metrics/throughput');
  },
};
