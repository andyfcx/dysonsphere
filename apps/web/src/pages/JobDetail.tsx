import { useState, useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useParams, Link } from 'react-router-dom'
import { api, Execution } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import TimeAgo from '../components/TimeAgo'

function LogBlock({ text }: { text: string }) {
  const preRef = useRef<HTMLPreElement>(null)
  useEffect(() => {
    if (preRef.current) {
      preRef.current.scrollTop = preRef.current.scrollHeight
    }
  }, [])
  return (
    <pre
      ref={preRef}
      style={{
        margin: 0,
        padding: '12px 16px',
        background: 'var(--bg3)',
        fontFamily: 'monospace',
        fontSize: 12,
        lineHeight: 1.6,
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-all',
        maxHeight: 400,
        overflowY: 'auto',
        color: 'var(--text)',
        borderTop: '1px solid var(--border)',
      }}
    >
      {text}
    </pre>
  )
}

export default function JobDetail() {
  const { id } = useParams<{ id: string }>()
  const { data, isLoading, error } = useQuery({
    queryKey: ['job', id],
    queryFn: () => api.getJob(id!),
    enabled: !!id,
  })
  const [expandedLogs, setExpandedLogs] = useState<Set<string>>(new Set())

  function toggleLog(execId: string) {
    setExpandedLogs(prev => {
      const next = new Set(prev)
      next.has(execId) ? next.delete(execId) : next.add(execId)
      return next
    })
  }

  if (isLoading) return <div className="loading">Loading...</div>
  if (error || !data) return <div className="error-msg">Job not found</div>

  const { job, executions = [] } = data

  return (
    <div>
      <Link to="/jobs" className="back-link">← Back to Jobs</Link>

      <div className="page-title">
        <span className="mono" style={{ fontSize: 18 }}>{job.normalized_command}</span>
        <div className="page-subtitle">{job.source_file}</div>
      </div>

      <div className="section">
        <div className="card">
          <div className="card-title">Job Details</div>
          <div className="detail-grid" style={{ marginTop: 8 }}>
            <span className="detail-label">Schedule</span>
            <span className="detail-value mono">{job.schedule || '—'}</span>

            <span className="detail-label">Timezone</span>
            <span className="detail-value">{job.timezone}</span>

            <span className="detail-label">User</span>
            <span className="detail-value">{job.user || '—'}</span>

            <span className="detail-label">Source Type</span>
            <span className="detail-value">
              <span className="badge badge-info">{job.source_type}</span>
            </span>

            <span className="detail-label">Raw Command</span>
            <span className="detail-value mono" style={{ fontSize: 12, wordBreak: 'break-all' }}>
              {job.raw_command}
            </span>

            <span className="detail-label">Command Hash</span>
            <span className="detail-value mono" style={{ fontSize: 12 }}>{job.command_hash}</span>

            <span className="detail-label">Enabled</span>
            <span className="detail-value">{job.enabled ? 'Yes' : 'No'}</span>

            <span className="detail-label">Discovered</span>
            <span className="detail-value"><TimeAgo iso={job.created_at} /></span>
          </div>
        </div>
      </div>

      <div className="section">
        <div className="section-title">Recent Executions</div>
        {executions.length === 0 ? (
          <div className="empty">No execution events recorded yet.</div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Status</th>
                  <th>Confidence</th>
                  <th>Started</th>
                  <th>Finished</th>
                  <th>Duration</th>
                  <th>Sources</th>
                  <th>Created</th>
                  <th>Log</th>
                </tr>
              </thead>
              <tbody>
                {executions.map((e: Execution) => {
                  const isOpen = expandedLogs.has(e.id)
                  return (
                    <>
                      <tr key={e.id}>
                        <td><StatusBadge status={e.status} /></td>
                        <td>
                          <span title="Confidence reflects inferred status, not exact measurement">
                            {(e.confidence_score * 100).toFixed(0)}%
                          </span>
                        </td>
                        <td><TimeAgo iso={e.detected_started_at} /></td>
                        <td><TimeAgo iso={e.detected_finished_at} fallback="—" /></td>
                        <td>{e.duration_seconds != null ? `${e.duration_seconds.toFixed(1)}s` : '—'}</td>
                        <td style={{ color: 'var(--text2)' }}>
                          {(e.detection_sources || []).join(', ') || '—'}
                        </td>
                        <td><TimeAgo iso={e.created_at} /></td>
                        <td>
                          {e.output_text ? (
                            <button
                              onClick={() => toggleLog(e.id)}
                              style={{
                                background: 'none',
                                border: '1px solid var(--border)',
                                borderRadius: 4,
                                color: 'var(--text2)',
                                cursor: 'pointer',
                                fontSize: 11,
                                padding: '2px 8px',
                              }}
                            >
                              {isOpen ? '▲ hide' : '▼ log'}
                            </button>
                          ) : '—'}
                        </td>
                      </tr>
                      {isOpen && e.output_text && (
                        <tr key={`${e.id}-log`}>
                          <td colSpan={8} style={{ padding: 0, borderBottom: '1px solid var(--border)' }}>
                            <LogBlock text={e.output_text} />
                          </td>
                        </tr>
                      )}
                    </>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="section">
        <div className="card" style={{ fontSize: 12, color: 'var(--text2)' }}>
          <div className="card-title">Observability Note</div>
          <p>
            Execution status is inferred from external observation (process detection, cron logs).
            Since this agent does not modify the original cronjob, exact exit codes are not available.
            The <strong>confidence_score</strong> reflects the quality of the inference.
          </p>
        </div>
      </div>
    </div>
  )
}
