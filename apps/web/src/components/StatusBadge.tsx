interface Props {
  status: string
}

const labelMap: Record<string, string> = {
  active: 'active', stale: 'stale', offline: 'offline', unknown: 'unknown',
  success: 'success', failed: 'failed', running: 'running', partial: 'partial', missed: 'missed',
  ok: 'ok', warning: 'warning', critical: 'critical',
  info: 'info',
}

export default function StatusBadge({ status }: Props) {
  const cls = labelMap[status] ?? 'unknown'
  return <span className={`badge badge-${cls}`}>{status}</span>
}
