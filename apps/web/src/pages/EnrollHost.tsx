import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, EnrollmentTokenCreated, EnrollmentTokenItem } from '../api/client'
import TimeAgo from '../components/TimeAgo'

export default function EnrollHost() {
  const queryClient = useQueryClient()
  const [label, setLabel] = useState('')
  const [generated, setGenerated] = useState<EnrollmentTokenCreated | null>(null)
  const [copied, setCopied] = useState(false)

  const { data: tokens = [], isLoading } = useQuery({
    queryKey: ['enrollment-tokens'],
    queryFn: api.listEnrollmentTokens,
  })

  const serverUrl = `${window.location.protocol}//${window.location.host}`

  const { mutate: generate, isPending } = useMutation({
    mutationFn: () => api.createEnrollmentToken(label.trim(), serverUrl),
    onSuccess: (data) => {
      setGenerated(data)
      setLabel('')
      queryClient.invalidateQueries({ queryKey: ['enrollment-tokens'] })
    },
  })

  function copyCommand() {
    if (!generated) return
    const cmd = `observer-agent init --payload ${generated.payload}`
    navigator.clipboard.writeText(cmd).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <div>
      <div className="page-title">
        Enroll New Host
        <div className="page-subtitle">Generate a one-time token to register a new agent</div>
      </div>

      <div style={{ maxWidth: 640, marginBottom: '2.5rem' }}>
        <div style={{ marginBottom: '1rem' }}>
          <label style={{ display: 'block', marginBottom: '0.4rem', fontWeight: 600, fontSize: 13 }}>
            Label <span style={{ color: 'var(--text2)', fontWeight: 400 }}>(optional)</span>
          </label>
          <input
            type="text"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="e.g. prod-web-01"
            style={{
              width: '100%',
              padding: '0.45rem 0.75rem',
              background: 'var(--bg2)',
              border: '1px solid var(--border)',
              borderRadius: 6,
              color: 'var(--text1)',
              fontSize: 14,
              boxSizing: 'border-box',
            }}
          />
        </div>
        <button
          onClick={() => generate()}
          disabled={isPending}
          style={{
            padding: '0.45rem 1.1rem',
            background: 'var(--accent)',
            color: '#fff',
            border: 'none',
            borderRadius: 6,
            cursor: isPending ? 'not-allowed' : 'pointer',
            fontSize: 14,
            fontWeight: 600,
            opacity: isPending ? 0.7 : 1,
          }}
        >
          {isPending ? 'Generating…' : 'Generate Token'}
        </button>
      </div>

      {generated && (
        <div style={{
          background: 'var(--bg2)',
          border: '1px solid var(--border)',
          borderRadius: 8,
          padding: '1.25rem',
          marginBottom: '2.5rem',
          maxWidth: 700,
        }}>
          <div style={{ fontWeight: 700, marginBottom: '0.75rem', fontSize: 14 }}>
            Run this command on the new host:
          </div>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
            background: 'var(--bg)',
            border: '1px solid var(--border)',
            borderRadius: 6,
            padding: '0.6rem 0.9rem',
            marginBottom: '0.75rem',
          }}>
            <code style={{
              flex: 1,
              fontSize: 12,
              wordBreak: 'break-all',
              color: 'var(--text1)',
              fontFamily: 'monospace',
            }}>
              observer-agent init --payload {generated.payload}
            </code>
            <button
              onClick={copyCommand}
              style={{
                flexShrink: 0,
                padding: '0.3rem 0.75rem',
                background: copied ? 'var(--success, #22c55e)' : 'var(--accent)',
                color: '#fff',
                border: 'none',
                borderRadius: 5,
                cursor: 'pointer',
                fontSize: 12,
                fontWeight: 600,
                transition: 'background 0.15s',
              }}
            >
              {copied ? 'Copied!' : 'Copy'}
            </button>
          </div>
          <div style={{ fontSize: 12, color: 'var(--text2)' }}>
            Token expires at {new Date(generated.expires_at).toLocaleString()}. Single use only.
          </div>
        </div>
      )}

      <div style={{ fontWeight: 700, fontSize: 14, marginBottom: '0.75rem' }}>
        Past tokens
      </div>

      {isLoading ? (
        <div className="loading">Loading…</div>
      ) : tokens.length === 0 ? (
        <div className="empty">No tokens generated yet.</div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Label</th>
                <th>Status</th>
                <th>Expires</th>
                <th>Created</th>
                <th>Used</th>
              </tr>
            </thead>
            <tbody>
              {tokens.map((t: EnrollmentTokenItem) => {
                const expired = !t.used && new Date(t.expires_at) < new Date()
                return (
                  <tr key={t.id}>
                    <td style={{ fontWeight: 600 }}>{t.label || <span style={{ color: 'var(--text2)' }}>—</span>}</td>
                    <td>
                      {t.used ? (
                        <span className="badge badge-info">used</span>
                      ) : expired ? (
                        <span className="badge" style={{ background: 'var(--text2)', color: '#fff' }}>expired</span>
                      ) : (
                        <span className="badge badge-success" style={{ background: 'var(--success, #22c55e)', color: '#fff' }}>active</span>
                      )}
                    </td>
                    <td><TimeAgo iso={t.expires_at} /></td>
                    <td><TimeAgo iso={t.created_at} /></td>
                    <td>{t.used_at ? <TimeAgo iso={t.used_at} /> : <span style={{ color: 'var(--text2)' }}>—</span>}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
