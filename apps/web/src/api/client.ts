const BASE = import.meta.env.VITE_API_BASE_URL || ''
const TOKEN = import.meta.env.VITE_API_TOKEN || 'dev-token'

async function apiFetch<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { Authorization: `Bearer ${TOKEN}` },
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`API ${path} → ${res.status}: ${body}`)
  }
  return res.json()
}

// ── Types ──────────────────────────────────────────────────────────────────

export interface Host {
  id: string
  machine_id: string
  hostname: string
  ip_address: string
  environment: string
  tags: string[]
  registered_at: string
  last_heartbeat_at?: string
  status: 'active' | 'stale' | 'offline' | 'unknown'
  agent_version: string
}

export interface Job {
  id: string
  host_id: string
  source_type: string
  schedule: string
  timezone: string
  user: string
  raw_command: string
  normalized_command: string
  command_hash: string
  enabled: boolean
  source_file: string
  created_at: string
  updated_at: string
}

export interface Execution {
  id: string
  host_id: string
  job_id?: string
  scheduled_at?: string
  detected_started_at?: string
  detected_finished_at?: string
  duration_seconds?: number
  status: string
  confidence_score: number
  detection_sources: string[]
  evidence?: Record<string, unknown>
  created_at: string
}

export interface DataMetric {
  id: string
  host_id: string
  metric_name: string
  metric_type: string
  dimensions?: Record<string, string>
  value: number
  measured_at: string
  status: string
  evidence?: Record<string, unknown>
  created_at: string
}

export interface Alert {
  id: string
  target_type: string
  target_id: string
  rule_name: string
  severity: string
  status: string
  message: string
  triggered_at: string
  resolved_at?: string
}

export interface Stats {
  total_hosts: number
  active_hosts: number
  total_jobs: number
  active_alerts: number
  recent_failed: number
}

export interface JobDetail {
  job: Job
  executions: Execution[]
}

// ── API calls ──────────────────────────────────────────────────────────────

export const api = {
  getStats: () => apiFetch<Stats>('/api/v1/stats'),
  getHosts: () => apiFetch<Host[]>('/api/v1/hosts'),
  getJobs: () => apiFetch<Job[]>('/api/v1/jobs'),
  getJob: (id: string) => apiFetch<JobDetail>(`/api/v1/jobs/${id}`),
  getExecutions: () => apiFetch<Execution[]>('/api/v1/executions'),
  getMetrics: () => apiFetch<DataMetric[]>('/api/v1/metrics'),
  getAlerts: () => apiFetch<Alert[]>('/api/v1/alerts'),
}
