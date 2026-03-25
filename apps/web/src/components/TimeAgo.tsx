import { formatDistanceToNow, parseISO } from 'date-fns'

interface Props {
  iso?: string
  fallback?: string
}

export default function TimeAgo({ iso, fallback = '—' }: Props) {
  if (!iso) return <span>{fallback}</span>
  try {
    return (
      <span title={iso}>
        {formatDistanceToNow(parseISO(iso), { addSuffix: true })}
      </span>
    )
  } catch {
    return <span>{iso}</span>
  }
}
