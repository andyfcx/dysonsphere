import { useQuery } from '@tanstack/react-query'
import { api, Alert } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import TimeAgo from '../components/TimeAgo'

export default function Alerts() {
  const { data: alerts = [], isLoading, error } = useQuery({
    queryKey: ['alerts'],
    queryFn: api.getAlerts,
  })

  if (isLoading) return <div className="loading">Loading alerts...</div>
  if (error) return <div className="error-msg">{String(error)}</div>

  const safeAlerts = alerts ?? []
  const active = safeAlerts.filter((a: Alert) => a.status === 'active')
  const resolved = safeAlerts.filter((a: Alert) => a.status === 'resolved')

  return (
    <div>
      <div className="page-title">
        Alerts
        <div className="page-subtitle">
          {active.length} active, {resolved.length} resolved
        </div>
      </div>

      {active.length > 0 && (
        <div className="section">
          <div className="section-title" style={{ color: 'var(--red)' }}>Active Alerts</div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Rule</th>
                  <th>Target</th>
                  <th>Severity</th>
                  <th>Message</th>
                  <th>Triggered</th>
                </tr>
              </thead>
              <tbody>
                {active.map((a: Alert) => <AlertRow key={a.id} alert={a} />)}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {safeAlerts.length === 0 && (
        <div className="empty" style={{ color: 'var(--green)' }}>
          ✓ No alerts. All systems operational.
        </div>
      )}

      {resolved.length > 0 && (
        <div className="section">
          <div className="section-title" style={{ color: 'var(--text2)' }}>Resolved</div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Rule</th>
                  <th>Target</th>
                  <th>Severity</th>
                  <th>Message</th>
                  <th>Triggered</th>
                  <th>Resolved</th>
                </tr>
              </thead>
              <tbody>
                {resolved.map((a: Alert) => <AlertRow key={a.id} alert={a} showResolved />)}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}

function AlertRow({ alert: a, showResolved }: { alert: Alert; showResolved?: boolean }) {
  return (
    <tr>
      <td style={{ fontWeight: 600 }}>{a.rule_name}</td>
      <td>
        <span style={{ color: 'var(--text2)', fontSize: 12 }}>{a.target_type}/</span>
        <span className="mono" style={{ fontSize: 12 }}>{a.target_id.slice(0, 8)}…</span>
      </td>
      <td><StatusBadge status={a.severity} /></td>
      <td style={{ maxWidth: 400 }}>{a.message}</td>
      <td><TimeAgo iso={a.triggered_at} /></td>
      {showResolved && <td><TimeAgo iso={a.resolved_at} fallback="—" /></td>}
    </tr>
  )
}
