const BASE = import.meta.env.VITE_API_BASE_URL || ''
const AUTH_TOKEN_KEY = 'observer.auth.token'
const AUTH_EVENT = 'observer-auth-changed'

function emitAuthChanged() {
  window.dispatchEvent(new Event(AUTH_EVENT))
}

export function getAuthToken(): string | null {
  return window.localStorage.getItem(AUTH_TOKEN_KEY)
}

export function setAuthToken(token: string) {
  window.localStorage.setItem(AUTH_TOKEN_KEY, token)
  emitAuthChanged()
}

export function clearAuthToken() {
  window.localStorage.removeItem(AUTH_TOKEN_KEY)
  emitAuthChanged()
}

export function subscribeAuthChanged(listener: () => void) {
  const handleStorage = (event: StorageEvent) => {
    if (event.key === AUTH_TOKEN_KEY) {
      listener()
    }
  }

  window.addEventListener(AUTH_EVENT, listener)
  window.addEventListener('storage', handleStorage)
  return () => {
    window.removeEventListener(AUTH_EVENT, listener)
    window.removeEventListener('storage', handleStorage)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getAuthToken()
  const headers = new Headers(init?.headers)
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers,
  })

  if (res.status === 401) {
    clearAuthToken()
  }
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`API ${path} -> ${res.status}: ${body}`)
  }
  return res.json()
}

export interface LoginResult {
  token: string
  username: string
}

export async function login(username: string, password: string): Promise<LoginResult> {
  return request<LoginResult>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export async function logout(): Promise<void> {
  const token = getAuthToken()
  if (!token) {
    clearAuthToken()
    return
  }

  try {
    await request<{ ok: boolean }>('/api/v1/auth/logout', {
      method: 'POST',
    })
  } finally {
    clearAuthToken()
  }
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

// ── Analytics types ────────────────────────────────────────────────────────

export interface WindowStats {
  window: string
  since: string
  total: number
  success: number
  failed: number
  unknown: number
  missed: number
  success_rate: number
}

export interface TrendPoint {
  bucket: string
  total: number
  success: number
  failed: number
  unknown: number
}

export interface JobSuccessRate {
  job_id: string
  host_id: string
  total: number
  success: number
  failed: number
  success_rate: number
  consecutive_fail: number
  last_run_at?: string
}

export interface HostFailureCount {
  host_id: string
  total: number
  failed: number
  failure_rate: number
}

export interface RecoveryEpisode {
  episode_start: string
  episode_end: string
  failure_count: number
  recovered_at?: string
  recovery_seconds?: number
}

export interface JobRecoverySummary {
  job_id: string
  host_id: string
  is_currently_failing: boolean
  current_episode_since?: string
  current_episode_fails?: number
  last_recovered_at?: string
  avg_recovery_seconds?: number
  episodes: RecoveryEpisode[]
}

export interface EnrollmentTokenCreated {
  id: string
  label: string
  payload: string
  expires_at: string
  created_at: string
}

export interface EnrollmentTokenItem {
  id: string
  label: string
  used: boolean
  used_at?: string
  expires_at: string
  created_at: string
}

// ── API calls ──────────────────────────────────────────────────────────────

export const api = {
  getStats: () => request<Stats>('/api/v1/stats'),
  getHosts: () => request<Host[]>('/api/v1/hosts'),
  getJobs: () => request<Job[]>('/api/v1/jobs'),
  getJob: (id: string) => request<JobDetail>(`/api/v1/jobs/${id}`),
  getExecutions: (params?: { status?: string; host_id?: string; job_id?: string; window?: string }) => {
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.host_id) q.set('host_id', params.host_id)
    if (params?.job_id) q.set('job_id', params.job_id)
    if (params?.window) q.set('window', params.window)
    const qs = q.toString()
    return request<Execution[]>(`/api/v1/executions${qs ? '?' + qs : ''}`)
  },
  // ── Analytics ──────────────────────────────────────────────────────────
  getWindowStats: (window?: string) =>
    request<WindowStats>(`/api/v1/stats/failures${window ? '?window=' + window : ''}`),
  getFailureTrend: (window?: string) =>
    request<{ trend: TrendPoint[] }>(`/api/v1/stats/trend${window ? '?window=' + window : ''}`),
  getJobStats: (window?: string) =>
    request<{ jobs: JobSuccessRate[] }>(`/api/v1/stats/jobs${window ? '?window=' + window : ''}`),
  getHostStats: (window?: string) =>
    request<{ hosts: HostFailureCount[] }>(`/api/v1/stats/hosts${window ? '?window=' + window : ''}`),
  getRecoveryStats: () =>
    request<{ jobs: JobRecoverySummary[] }>('/api/v1/stats/recovery'),
  getMetrics: () => request<DataMetric[]>('/api/v1/metrics'),
  getAlerts: () => request<Alert[]>('/api/v1/alerts'),
  createEnrollmentToken: (label: string, serverUrl: string) =>
    request<EnrollmentTokenCreated>('/api/v1/enrollment-tokens', {
      method: 'POST',
      body: JSON.stringify({ label, server_url: serverUrl }),
    }),
  listEnrollmentTokens: () => request<EnrollmentTokenItem[]>('/api/v1/enrollment-tokens'),
}
