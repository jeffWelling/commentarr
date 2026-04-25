import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'

type Job = {
  id: number
  client_name: string
  client_job_id: string
  title_id: string
  release_title: string
  edition: string
  status: string
  outcome: string
  added_at: string
  imported_at?: string | null
}

const statusBadge: Record<string, string> = {
  queued: 'bg-[color:var(--bg-2)] text-[color:var(--fg-1)]',
  completed: 'bg-[color:var(--accent)]/15 text-[color:var(--accent)]',
  imported: 'bg-[color:var(--success)]/15 text-[color:var(--success)]',
  error: 'bg-[color:var(--error)]/15 text-[color:var(--error)]',
}

function StatusPill({ status }: { status: string }) {
  const cls = statusBadge[status] ?? 'bg-[color:var(--bg-2)] text-[color:var(--fg-2)]'
  return <span className={`mono text-xs px-2 py-0.5 rounded-md ${cls}`}>{status}</span>
}

export function Downloads() {
  const q = useQuery<{ jobs: Job[] | null }>({
    queryKey: ['jobs'],
    queryFn: () => api.get('/api/v1/jobs/?limit=100'),
    refetchInterval: 5_000,
  })

  const jobs = q.data?.jobs ?? []

  return (
    <div className="space-y-6 max-w-6xl">
      <header className="flex items-baseline justify-between">
        <h1 className="text-3xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
          downloads
        </h1>
        <span className="text-xs text-[color:var(--fg-2)]">
          {jobs.length} {jobs.length === 1 ? 'job' : 'jobs'} · auto-refresh 5s
        </span>
      </header>

      {q.isLoading && <p className="text-[color:var(--fg-2)]">loading…</p>}
      {!q.isLoading && jobs.length === 0 && (
        <p className="text-[color:var(--fg-2)]">
          nothing here yet — when the picker queues a download, it'll appear above
        </p>
      )}

      {jobs.length > 0 && (
        <div className="overflow-x-auto rounded-xl border border-[color:var(--bg-2)] bg-[color:var(--bg-1)]">
          <table className="w-full text-sm">
            <thead className="text-left text-xs uppercase tracking-widest text-[color:var(--fg-2)] border-b border-[color:var(--bg-2)]">
              <tr>
                <th className="px-4 py-3">release</th>
                <th className="px-4 py-3">title id</th>
                <th className="px-4 py-3">client</th>
                <th className="px-4 py-3">status</th>
                <th className="px-4 py-3">outcome</th>
                <th className="px-4 py-3">added</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((j) => (
                <tr key={j.id} className="border-t border-[color:var(--bg-2)]">
                  <td className="px-4 py-3 mono text-xs">
                    <div className="break-all">{j.release_title || j.client_job_id}</div>
                    {j.edition && (
                      <div className="text-[color:var(--fg-2)]">edition: {j.edition}</div>
                    )}
                  </td>
                  <td className="px-4 py-3 mono text-xs text-[color:var(--fg-2)]">{j.title_id}</td>
                  <td className="px-4 py-3 mono text-xs">{j.client_name}</td>
                  <td className="px-4 py-3"><StatusPill status={j.status} /></td>
                  <td className="px-4 py-3 mono text-xs text-[color:var(--fg-2)]">{j.outcome || '—'}</td>
                  <td className="px-4 py-3 text-xs text-[color:var(--fg-2)]">
                    {new Date(j.added_at).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
