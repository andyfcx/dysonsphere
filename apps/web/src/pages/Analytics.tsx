import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip,
  ResponsiveContainer, Legend,
} from 'recharts'
import { formatDistanceToNow, format, parseISO } from 'date-fns'
import { api, type Host, type Job } from '../api/client'

type Window = '24h' | '7d' | '30d'

function fmtSeconds(s: number): string {
  if (s < 60) return `${Math.round(s)}s`
  if (s < 3600) return `${Math.round(s / 60)}m`
  if (s < 86400) return `${(s / 3600).toFixed(1)}h`
  return `${(s / 86400).toFixed(1)}d`
}

function pct(n: number): string {
  return n.toFixed(1) + '%'
}

// Map host/job UUIDs to display names for the tables.
function hostLabel(hosts: Host[], id: string): string {
  const h = hosts.find(h => h.id === id)
  return h ? h.hostname : id.slice(0, 8)
}

function jobLabel(jobs: Job[], id: string): string {
  const j = jobs.find(j => j.id === id)
  if (!j) return id.slice(0, 8)
  const cmd = j.raw_command.length > 40 ? j.raw_command.slice(0, 40) + '…' : j.raw_command
  return cmd
}

export default function Analytics() {
  const [win, setWin] = useState<Window>('24h')

  const { data: summary, isLoading: loadSum } = useQuery({
    queryKey: ['stats/failures', win],
    queryFn: () => api.getWindowStats(win),
  })
  const { data: trendData, isLoading: loadTrend } = useQuery({
    queryKey: ['stats/trend', win],
    queryFn: () => api.getFailureTrend(win),
  })
  const { data: jobData, isLoading: loadJobs } = useQuery({
    queryKey: ['stats/jobs', win],
    queryFn: () => api.getJobStats(win),
  })
  const { data: hostData, isLoading: loadHosts } = useQuery({
    queryKey: ['stats/hosts', win],
    queryFn: () => api.getHostStats(win),
  })
  const { data: recoveryData, isLoading: loadRecovery } = useQuery({
    queryKey: ['stats/recovery'],
    queryFn: () => api.getRecoveryStats(),
  })
  const { data: hosts = [] } = useQuery({ queryKey: ['hosts'], queryFn: api.getHosts })
  const { data: jobs = [] } = useQuery({ queryKey: ['jobs'], queryFn: api.getJobs })

  const trend = (trendData?.trend ?? []).map(p => ({
    ...p,
    label: format(parseISO(p.bucket), win === '30d' ? 'MM/dd' : win === '7d' ? 'MM/dd HH:mm' : 'HH:mm'),
  }))

  return (
    <div>
      <div className="page-title">
        Analytics
        <div className="page-subtitle">Failure statistics and recovery analysis</div>
      </div>

      {/* Window selector */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 20 }}>
        {(['24h', '7d', '30d'] as Window[]).map(w => (
          <button
            key={w}
            type="button"
            onClick={() => setWin(w)}
            style={{
              padding: '4px 14px',
              borderRadius: 6,
              border: '1px solid var(--border)',
              background: win === w ? 'var(--accent)' : 'var(--surface)',
              color: win === w ? '#fff' : 'var(--text1)',
              cursor: 'pointer',
              fontSize: 13,
            }}
          >
            {w}
          </button>
        ))}
      </div>

      {/* Summary cards */}
      {loadSum ? (
        <div className="loading">Loading…</div>
      ) : summary ? (
        <div className="stat-grid" style={{ marginBottom: 24 }}>
          <div className="stat-card">
            <div className="stat-label">Total Executions</div>
            <div className="stat-value">{summary.total}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">Success</div>
            <div className="stat-value green">{summary.success}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">Failed</div>
            <div className="stat-value" style={{ color: summary.failed > 0 ? 'var(--red)' : 'var(--green)' }}>
              {summary.failed}
            </div>
          </div>
          <div className="stat-card">
            <div className="stat-label">Unknown / Missed</div>
            <div className="stat-value" style={{ color: (summary.unknown + summary.missed) > 0 ? 'var(--yellow)' : 'var(--text2)' }}>
              {summary.unknown + summary.missed}
            </div>
          </div>
          <div className="stat-card">
            <div className="stat-label">Success Rate</div>
            <div className="stat-value" style={{ color: summary.success_rate >= 95 ? 'var(--green)' : summary.success_rate >= 80 ? 'var(--yellow)' : 'var(--red)' }}>
              {pct(summary.success_rate)}
            </div>
          </div>
        </div>
      ) : null}

      {/* Trend chart */}
      <div className="card" style={{ marginBottom: 24 }}>
        <div className="card-title">Execution Trend ({win})</div>
        {loadTrend ? (
          <div className="loading">Loading…</div>
        ) : trend.length === 0 ? (
          <div style={{ color: 'var(--text2)', fontSize: 13 }}>No execution data for this window.</div>
        ) : (
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={trend} margin={{ top: 4, right: 8, left: -16, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
              <XAxis dataKey="label" tick={{ fontSize: 11, fill: 'var(--text2)' }} />
              <YAxis tick={{ fontSize: 11, fill: 'var(--text2)' }} allowDecimals={false} />
              <Tooltip
                contentStyle={{ background: 'var(--surface)', border: '1px solid var(--border)', fontSize: 12 }}
                labelStyle={{ color: 'var(--text1)' }}
              />
              <Legend wrapperStyle={{ fontSize: 12 }} />
              <Bar dataKey="success" name="Success" fill="#4caf50" stackId="a" />
              <Bar dataKey="unknown" name="Unknown" fill="#888" stackId="a" />
              <Bar dataKey="failed" name="Failed" fill="#f44336" stackId="a" />
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 24 }}>
        {/* Per-job success rates */}
        <div className="card">
          <div className="card-title">Job Success Rates ({win})</div>
          {loadJobs ? (
            <div className="loading">Loading…</div>
          ) : !jobData?.jobs?.length ? (
            <div style={{ color: 'var(--text2)', fontSize: 13 }}>No data.</div>
          ) : (
            <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ color: 'var(--text2)', textAlign: 'left' }}>
                  <th style={{ paddingBottom: 6 }}>Job</th>
                  <th style={{ paddingBottom: 6, textAlign: 'right' }}>Total</th>
                  <th style={{ paddingBottom: 6, textAlign: 'right' }}>Failed</th>
                  <th style={{ paddingBottom: 6, textAlign: 'right' }}>Rate</th>
                  <th style={{ paddingBottom: 6, textAlign: 'right' }}>Consec.</th>
                </tr>
              </thead>
              <tbody>
                {jobData.jobs.map(j => (
                  <tr key={j.job_id} style={{ borderTop: '1px solid var(--border)' }}>
                    <td style={{ padding: '5px 0', maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      <span title={jobLabel(jobs, j.job_id)}>{jobLabel(jobs, j.job_id)}</span>
                      <div style={{ color: 'var(--text2)', fontSize: 11 }}>{hostLabel(hosts, j.host_id)}</div>
                    </td>
                    <td style={{ padding: '5px 0', textAlign: 'right' }}>{j.total}</td>
                    <td style={{ padding: '5px 0', textAlign: 'right', color: j.failed > 0 ? 'var(--red)' : undefined }}>{j.failed}</td>
                    <td style={{ padding: '5px 0', textAlign: 'right', color: j.success_rate >= 95 ? 'var(--green)' : j.success_rate >= 80 ? 'var(--yellow)' : 'var(--red)' }}>
                      {pct(j.success_rate)}
                    </td>
                    <td style={{ padding: '5px 0', textAlign: 'right', color: j.consecutive_fail > 0 ? 'var(--red)' : undefined }}>
                      {j.consecutive_fail > 0 ? j.consecutive_fail : '–'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        {/* Per-host failure counts */}
        <div className="card">
          <div className="card-title">Host Failure Counts ({win})</div>
          {loadHosts ? (
            <div className="loading">Loading…</div>
          ) : !hostData?.hosts?.length ? (
            <div style={{ color: 'var(--text2)', fontSize: 13 }}>No data.</div>
          ) : (
            <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ color: 'var(--text2)', textAlign: 'left' }}>
                  <th style={{ paddingBottom: 6 }}>Host</th>
                  <th style={{ paddingBottom: 6, textAlign: 'right' }}>Total</th>
                  <th style={{ paddingBottom: 6, textAlign: 'right' }}>Failed</th>
                  <th style={{ paddingBottom: 6, textAlign: 'right' }}>Fail%</th>
                </tr>
              </thead>
              <tbody>
                {hostData.hosts.map(h => (
                  <tr key={h.host_id} style={{ borderTop: '1px solid var(--border)' }}>
                    <td style={{ padding: '5px 0' }}>{hostLabel(hosts, h.host_id)}</td>
                    <td style={{ padding: '5px 0', textAlign: 'right' }}>{h.total}</td>
                    <td style={{ padding: '5px 0', textAlign: 'right', color: h.failed > 0 ? 'var(--red)' : undefined }}>{h.failed}</td>
                    <td style={{ padding: '5px 0', textAlign: 'right', color: h.failure_rate >= 20 ? 'var(--red)' : h.failure_rate > 0 ? 'var(--yellow)' : undefined }}>
                      {pct(h.failure_rate)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* Recovery analysis */}
      <div className="card">
        <div className="card-title">Recovery Analysis</div>
        <div style={{ color: 'var(--text2)', fontSize: 12, marginBottom: 12 }}>
          Consecutive failure episodes and time-to-recovery per job (all history).
        </div>
        {loadRecovery ? (
          <div className="loading">Loading…</div>
        ) : !recoveryData?.jobs?.length ? (
          <div style={{ color: 'var(--text2)', fontSize: 13 }}>No failure episodes recorded.</div>
        ) : (
          <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ color: 'var(--text2)', textAlign: 'left' }}>
                <th style={{ paddingBottom: 6 }}>Job</th>
                <th style={{ paddingBottom: 6 }}>Status</th>
                <th style={{ paddingBottom: 6, textAlign: 'right' }}>Episodes</th>
                <th style={{ paddingBottom: 6, textAlign: 'right' }}>Current fails</th>
                <th style={{ paddingBottom: 6, textAlign: 'right' }}>Failing since</th>
                <th style={{ paddingBottom: 6, textAlign: 'right' }}>Avg recovery</th>
                <th style={{ paddingBottom: 6, textAlign: 'right' }}>Last recovered</th>
              </tr>
            </thead>
            <tbody>
              {recoveryData.jobs.map(j => (
                <tr key={j.job_id} style={{ borderTop: '1px solid var(--border)' }}>
                  <td style={{ padding: '6px 0', maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    <span title={jobLabel(jobs, j.job_id)}>{jobLabel(jobs, j.job_id)}</span>
                    <div style={{ color: 'var(--text2)', fontSize: 11 }}>{hostLabel(hosts, j.host_id)}</div>
                  </td>
                  <td style={{ padding: '6px 0' }}>
                    {j.is_currently_failing ? (
                      <span style={{ color: 'var(--red)', fontWeight: 600 }}>● Failing</span>
                    ) : (
                      <span style={{ color: 'var(--green)' }}>✓ OK</span>
                    )}
                  </td>
                  <td style={{ padding: '6px 0', textAlign: 'right' }}>{j.episodes.length}</td>
                  <td style={{ padding: '6px 0', textAlign: 'right', color: j.current_episode_fails ? 'var(--red)' : undefined }}>
                    {j.is_currently_failing ? j.current_episode_fails : '–'}
                  </td>
                  <td style={{ padding: '6px 0', textAlign: 'right', fontSize: 11 }}>
                    {j.current_episode_since
                      ? formatDistanceToNow(parseISO(j.current_episode_since), { addSuffix: true })
                      : '–'}
                  </td>
                  <td style={{ padding: '6px 0', textAlign: 'right' }}>
                    {j.avg_recovery_seconds != null ? fmtSeconds(j.avg_recovery_seconds) : '–'}
                  </td>
                  <td style={{ padding: '6px 0', textAlign: 'right', fontSize: 11 }}>
                    {j.last_recovered_at
                      ? formatDistanceToNow(parseISO(j.last_recovered_at), { addSuffix: true })
                      : '–'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {/* Per-job episode drill-down */}
        {recoveryData?.jobs?.some(j => j.episodes.length > 0) && (
          <details style={{ marginTop: 20 }}>
            <summary style={{ cursor: 'pointer', fontSize: 12, color: 'var(--text2)' }}>
              Episode detail (expand)
            </summary>
            {recoveryData.jobs.map(j => (
              <div key={j.job_id} style={{ marginTop: 12 }}>
                <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 4 }}>
                  {jobLabel(jobs, j.job_id)}
                  <span style={{ color: 'var(--text2)', fontWeight: 400, marginLeft: 6 }}>{hostLabel(hosts, j.host_id)}</span>
                </div>
                <table style={{ width: '100%', fontSize: 11, borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ color: 'var(--text2)' }}>
                      <th style={{ textAlign: 'left', paddingBottom: 4 }}>Episode start</th>
                      <th style={{ textAlign: 'right', paddingBottom: 4 }}>Failures</th>
                      <th style={{ textAlign: 'right', paddingBottom: 4 }}>Recovered at</th>
                      <th style={{ textAlign: 'right', paddingBottom: 4 }}>Time to recover</th>
                    </tr>
                  </thead>
                  <tbody>
                    {j.episodes.map((ep, i) => (
                      <tr key={i} style={{ borderTop: '1px solid var(--border)' }}>
                        <td style={{ padding: '4px 0' }}>{format(parseISO(ep.episode_start), 'yyyy-MM-dd HH:mm')}</td>
                        <td style={{ padding: '4px 0', textAlign: 'right', color: 'var(--red)' }}>{ep.failure_count}</td>
                        <td style={{ padding: '4px 0', textAlign: 'right' }}>
                          {ep.recovered_at ? format(parseISO(ep.recovered_at), 'yyyy-MM-dd HH:mm') : <span style={{ color: 'var(--red)' }}>Still failing</span>}
                        </td>
                        <td style={{ padding: '4px 0', textAlign: 'right' }}>
                          {ep.recovery_seconds != null ? fmtSeconds(ep.recovery_seconds) : '–'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ))}
          </details>
        )}
      </div>
    </div>
  )
}
