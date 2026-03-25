import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'

export default function Overview() {
  const { data: stats, isLoading, error } = useQuery({
    queryKey: ['stats'],
    queryFn: api.getStats,
  })

  if (isLoading) return <div className="loading">Loading...</div>
  if (error) return <div className="error-msg">Failed to load stats: {String(error)}</div>

  const s = stats!

  return (
    <div>
      <div className="page-title">
        Overview
        <div className="page-subtitle">System-wide status summary</div>
      </div>

      <div className="stat-grid">
        <div className="stat-card">
          <div className="stat-label">Total Hosts</div>
          <div className="stat-value">{s.total_hosts}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Active Hosts</div>
          <div className="stat-value green">{s.active_hosts}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Total Jobs</div>
          <div className="stat-value">{s.total_jobs}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Active Alerts</div>
          <div className="stat-value" style={{ color: s.active_alerts > 0 ? 'var(--red)' : 'var(--green)' }}>
            {s.active_alerts}
          </div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Recent Failures</div>
          <div className="stat-value" style={{ color: s.recent_failed > 0 ? 'var(--yellow)' : 'var(--green)' }}>
            {s.recent_failed}
          </div>
        </div>
      </div>

      <div className="card">
        <div className="card-title">System Health</div>
        <p style={{ color: 'var(--text2)', fontSize: 13 }}>
          {s.active_alerts === 0
            ? '✓ No active alerts. All systems nominal.'
            : `⚠ ${s.active_alerts} active alert${s.active_alerts > 1 ? 's' : ''} require attention.`}
        </p>
      </div>
    </div>
  )
}
