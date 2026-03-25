import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, Job } from '../api/client'
import TimeAgo from '../components/TimeAgo'

export default function Jobs() {
  const { data: jobs = [], isLoading, error } = useQuery({
    queryKey: ['jobs'],
    queryFn: api.getJobs,
  })

  if (isLoading) return <div className="loading">Loading jobs...</div>
  if (error) return <div className="error-msg">{String(error)}</div>

  return (
    <div>
      <div className="page-title">
        Jobs
        <div className="page-subtitle">{jobs.length} discovered jobs</div>
      </div>

      {jobs.length === 0 ? (
        <div className="empty">No jobs discovered yet.</div>
      ) : (
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
              {jobs.map((j: Job) => (
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
      )}
    </div>
  )
}

function truncate(s: string, n: number) {
  return s.length > n ? s.slice(0, n) + '…' : s
}
