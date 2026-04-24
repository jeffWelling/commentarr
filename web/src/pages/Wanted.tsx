import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'

type Candidate = {
  Score: number
  LikelyCommentary: boolean
  Release: { InfoHash: string; URL: string; Title: string; Seeders: number; Indexer: string }
  Reasons: Array<{ Rule: string; Score: number }>
}
type WantedRow = { title_id: string; candidates: Candidate[]; search_misses: number }

export function Wanted() {
  const qc = useQueryClient()
  const q = useQuery<{ wanted: WantedRow[] }>({
    queryKey: ['wanted'],
    queryFn: () => api.get('/api/v1/wanted/'),
  })
  const skip = useMutation({
    mutationFn: (id: string) => api.post<void>(`/api/v1/wanted/${encodeURIComponent(id)}/skip`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['wanted'] }),
  })

  const rows = q.data?.wanted ?? []

  return (
    <div className="space-y-6 max-w-5xl">
      <h1 className="text-3xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
        wanted ({rows.length})
      </h1>

      {q.isLoading && <p className="text-[color:var(--fg-2)]">loading…</p>}
      {q.error && <p className="text-[color:var(--error)]">error loading wanted list</p>}

      <ul className="space-y-4">
        {rows.map((r) => (
          <li
            key={r.title_id}
            className="rounded-xl border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-5"
          >
            <div className="flex items-baseline justify-between">
              <div>
                <div className="mono text-[color:var(--fg-0)] text-lg">{r.title_id}</div>
                <div className="text-xs text-[color:var(--fg-2)]">
                  {r.search_misses} search {r.search_misses === 1 ? 'miss' : 'misses'} so far
                </div>
              </div>
              <button
                onClick={() => skip.mutate(r.title_id)}
                className="text-sm text-[color:var(--fg-2)] hover:text-[color:var(--error)]"
                disabled={skip.isPending}
              >
                skip
              </button>
            </div>
            {r.candidates.length > 0 && (
              <div className="mt-4 space-y-2">
                {r.candidates.map((c, i) => (
                  <div
                    key={c.Release.InfoHash + i}
                    className="rounded-lg border border-[color:var(--bg-2)] p-3 bg-[color:var(--bg-0)]"
                  >
                    <div className="flex items-baseline gap-3">
                      <span
                        className={
                          'font-bold text-lg ' +
                          (c.LikelyCommentary
                            ? 'text-[color:var(--success)]'
                            : 'text-[color:var(--fg-2)]')
                        }
                      >
                        {c.Score}
                      </span>
                      <span className="mono flex-1 text-sm">{c.Release.Title}</span>
                      <span className="text-xs text-[color:var(--fg-2)]">
                        {c.Release.Seeders} seeders · {c.Release.Indexer}
                      </span>
                    </div>
                    {c.Reasons?.length > 0 && (
                      <div className="mt-2 text-xs text-[color:var(--fg-2)]">
                        {c.Reasons.map((r) => `${r.Rule}(+${r.Score})`).join(' ')}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}
