import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'

type UpgradeInfo = {
  TitleID: string
  CurrentRelease: string
  CurrentScore: number
  CandidateRelease: string
  CandidateScore: number
  CandidateIndexer: string
  CandidateInfoHash: string
}

export function Upgrades() {
  const q = useQuery<{ upgrades: UpgradeInfo[] }>({
    queryKey: ['upgrades'],
    queryFn: () => api.get('/api/v1/upgrades/'),
    refetchInterval: 60_000,
  })
  const rows = q.data?.upgrades ?? []

  return (
    <div className="space-y-6 max-w-5xl">
      <h1
        className="text-3xl font-bold"
        style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}
      >
        upgrades ({rows.length})
      </h1>
      <p className="text-sm text-[color:var(--fg-2)] max-w-2xl">
        Resolved titles whose periodic re-search turned up a release scoring
        higher than the one currently in your library. Commentarr never
        auto-replaces a successfully-imported file — these are advisories
        for you to review and (optionally) re-grab.
      </p>

      {rows.length === 0 && !q.isLoading && (
        <p className="text-sm text-[color:var(--fg-2)]">
          no upgrades available right now — check back after the next recheck cycle (default 6 months per title).
        </p>
      )}

      <div className="space-y-3">
        {rows.map((u) => (
          <div
            key={u.TitleID}
            className="rounded-lg border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-4"
          >
            <div className="flex items-baseline justify-between gap-4">
              <span className="mono text-xs text-[color:var(--fg-2)]">{u.TitleID}</span>
              <span className="mono text-xs text-[color:var(--accent)]">
                +{u.CandidateScore - u.CurrentScore} score
              </span>
            </div>
            <div className="mt-2 space-y-1 mono text-sm">
              <div>
                <span className="text-[color:var(--fg-2)]">have:&nbsp;</span>
                <span>{u.CurrentRelease}</span>
                <span className="text-[color:var(--fg-2)]"> ({u.CurrentScore})</span>
              </div>
              <div>
                <span className="text-[color:var(--fg-2)]">avail:&nbsp;</span>
                <span className="text-[color:var(--accent)]">{u.CandidateRelease}</span>
                <span className="text-[color:var(--fg-2)]">
                  &nbsp;({u.CandidateScore}) — {u.CandidateIndexer}
                </span>
              </div>
              {u.CandidateInfoHash && (
                <div className="text-xs text-[color:var(--fg-2)] mt-1 break-all">
                  magnet:?xt=urn:btih:{u.CandidateInfoHash}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
