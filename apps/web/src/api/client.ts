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

// ── API calls ──────────────────────────────────────────────────────────────

export const api = {
  getStats: () => request<Stats>('/api/v1/stats'),
  getHosts: () => request<Host[]>('/api/v1/hosts'),
  getJobs: () => request<Job[]>('/api/v1/jobs'),
  getJob: (id: string) => request<JobDetail>(`/api/v1/jobs/${id}`),
  getExecutions: () => request<Execution[]>('/api/v1/executions'),
  getMetrics: () => request<DataMetric[]>('/api/v1/metrics'),
  getAlerts: () => request<Alert[]>('/api/v1/alerts'),
}
