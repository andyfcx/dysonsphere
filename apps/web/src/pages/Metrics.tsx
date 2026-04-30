import { useQuery } from '@tanstack/react-query'
import { api, DataMetric } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import TimeAgo from '../components/TimeAgo'

export default function Metrics() {
  const { data: metrics = [], isLoading, error } = useQuery({
    queryKey: ['metrics'],
    queryFn: api.getMetrics,
  })

  if (isLoading) return <div className="loading">Loading metrics...</div>
  if (error) return <div className="error-msg">{String(error)}</div>

  const safeMetrics = metrics ?? []

  return (
    <div>
      <div className="page-title">
        Data Metrics
        <div className="page-subtitle">Probe results from all agents</div>
      </div>

      {safeMetrics.length === 0 ? (
        <div className="empty">No metric data yet. Configure probes in agent config.</div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Metric Name</th>
                <th>Type</th>
                <th>Value</th>
                <th>Status</th>
                <th>Dimensions</th>
                <th>Measured</th>
              </tr>
            </thead>
            <tbody>
              {safeMetrics.map((m: DataMetric) => (
                <tr key={m.id}>
                  <td style={{ fontWeight: 600 }}>{m.metric_name}</td>
                  <td>
                    <span className="badge badge-info">{m.metric_type}</span>
                  </td>
                  <td className="mono" style={{ fontWeight: 600 }}>
                    {formatValue(m.value, m.metric_type)}
                  </td>
                  <td><StatusBadge status={m.status} /></td>
                  <td style={{ color: 'var(--text2)', fontSize: 12 }}>
                    {m.dimensions ? formatDimensions(m.dimensions) : '—'}
                  </td>
                  <td><TimeAgo iso={m.measured_at} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function formatValue(v: number, type: string): string {
  if (type === 'boolean') return v > 0 ? 'true' : 'false'
  if (type === 'count') return v.toLocaleString()
  return v.toFixed(2)
}

function formatDimensions(dims: Record<string, string>): string {
  return Object.entries(dims).map(([k, v]) => `${k}=${v}`).join(', ')
}
