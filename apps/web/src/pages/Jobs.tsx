import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, Host, Job } from '../api/client'
import TimeAgo from '../components/TimeAgo'

export default function Jobs() {
  const { data: jobs = [], isLoading: jobsLoading, error: jobsError } = useQuery({
    queryKey: ['jobs'],
    queryFn: api.getJobs,
  })

  const { data: hosts = [], isLoading: hostsLoading } = useQuery({
    queryKey: ['hosts'],
    queryFn: api.getHosts,
  })

  if (jobsLoading || hostsLoading) return <div className="loading">Loading jobs...</div>
  if (jobsError) return <div className="error-msg">{String(jobsError)}</div>

  const hostMap = new Map<string, Host>(hosts.map((h: Host) => [h.id, h]))

  const groups = jobs.reduce<Map<string, Job[]>>((acc, job: Job) => {
    const list = acc.get(job.host_id) ?? []
    list.push(job)
    acc.set(job.host_id, list)
    return acc
  }, new Map())

  const hostIds = Array.from(groups.keys()).sort((a, b) => {
    const ha = hostMap.get(a)?.hostname ?? a
    const hb = hostMap.get(b)?.hostname ?? b
    return ha.localeCompare(hb)
  })

  return (
    <div>
      <div className="page-title">
        Jobs
        <div className="page-subtitle">{jobs.length} discovered jobs across {hostIds.length} host{hostIds.length !== 1 ? 's' : ''}</div>
      </div>

      {jobs.length === 0 ? (
        <div className="empty">No jobs discovered yet.</div>
      ) : (
        hostIds.map((hostId) => {
          const host = hostMap.get(hostId)
          const hostJobs = groups.get(hostId) ?? []
          return (
            <div key={hostId} style={{ marginBottom: '2rem' }}>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: '0.5rem', marginBottom: '0.5rem' }}>
                <span style={{ fontWeight: 700, fontSize: 15 }}>{host?.hostname ?? hostId}</span>
                {host && (
                  <span className="mono" style={{ color: 'var(--text2)', fontSize: 11 }}>{host.ip_address || host.machine_id.slice(0, 12) + '…'}</span>
                )}
                <span style={{ color: 'var(--text2)', fontSize: 12 }}>{hostJobs.length} job{hostJobs.length !== 1 ? 's' : ''}</span>
              </div>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Command</th>
                      <th>Schedule</th>
                      <th>Timezone</th>
                      <th>User</th>
                      <th>Source</th>
                      <th>Enabled</th>
                      <th>Discovered</th>
                    </tr>
                  </thead>
                  <tbody>
                    {hostJobs.map((j: Job) => (
                      <tr key={j.id}>
                        <td>
                          <Link to={`/jobs/${j.id}`} className="mono" style={{ fontWeight: 600 }}>
                            {truncate(j.normalized_command, 60)}
                          </Link>
                          <br />
                          <span style={{ color: 'var(--text2)', fontSize: 11 }}>{j.source_file}</span>
                        </td>
                        <td className="mono">{j.schedule || '—'}</td>
                        <td>{j.timezone}</td>
                        <td>{j.user || '—'}</td>
                        <td>
                          <span className="badge badge-info">{j.source_type}</span>
                        </td>
                        <td>
                          <span className={`dot ${j.enabled ? 'dot-green' : 'dot-red'}`} />
                          {' '}{j.enabled ? 'yes' : 'no'}
                        </td>
                        <td><TimeAgo iso={j.created_at} /></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )
        })
      )}
    </div>
  )
}

function truncate(s: string, n: number) {
  return s.length > n ? s.slice(0, n) + '…' : s
}
