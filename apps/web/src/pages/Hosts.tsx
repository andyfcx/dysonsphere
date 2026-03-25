import { useQuery } from '@tanstack/react-query'
import { api, Host } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import TimeAgo from '../components/TimeAgo'

export default function Hosts() {
  const { data: hosts = [], isLoading, error } = useQuery({
    queryKey: ['hosts'],
    queryFn: api.getHosts,
  })

  if (isLoading) return <div className="loading">Loading hosts...</div>
  if (error) return <div className="error-msg">{String(error)}</div>

  return (
    <div>
      <div className="page-title">Hosts</div>

      {hosts.length === 0 ? (
        <div className="empty">No hosts registered yet. Run <code>observer-agent init</code> on a host.</div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Hostname</th>
                <th>Environment</th>
                <th>Status</th>
                <th>Last Heartbeat</th>
                <th>Tags</th>
                <th>IP</th>
                <th>Agent</th>
              </tr>
            </thead>
            <tbody>
              {hosts.map((h: Host) => (
                <tr key={h.id}>
                  <td>
                    <span style={{ fontWeight: 600 }}>{h.hostname}</span>
                    <br />
                    <span className="mono" style={{ color: 'var(--text2)', fontSize: 11 }}>
                      {h.machine_id.slice(0, 12)}…
                    </span>
                  </td>
                  <td>{h.environment}</td>
                  <td><StatusBadge status={h.status} /></td>
                  <td><TimeAgo iso={h.last_heartbeat_at} fallback="never" /></td>
                  <td>
                    {(h.tags || []).map((t: string) => (
                      <span key={t} className="tag">{t}</span>
                    ))}
                  </td>
                  <td className="mono">{h.ip_address || '—'}</td>
                  <td style={{ color: 'var(--text2)' }}>{h.agent_version}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
