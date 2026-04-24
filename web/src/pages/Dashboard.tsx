import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '../api/client'
import { useEvents } from '../api/useEvents'
import type { SSEMessage } from '../api/useEvents'

type TitleRow = { ID: string; Kind: string; DisplayName: string; Year: number; FilePath: string }
type WantedRow = { title_id: string; candidates: Array<unknown>; search_misses: number }

function Stat({ label, value, hint }: { label: string; value: string | number; hint?: string }) {
  return (
    <div className="rounded-xl border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-6">
      <div className="text-xs uppercase tracking-widest text-[color:var(--fg-2)]">{label}</div>
      <div className="mt-2 text-4xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
        {value}
      </div>
      {hint && <div className="mt-1 text-xs text-[color:var(--fg-2)]">{hint}</div>}
    </div>
  )
}

export function Dashboard() {
  const titlesQ = useQuery<{ titles: TitleRow[] }>({
    queryKey: ['titles'],
    queryFn: () => api.get('/api/v1/library/titles'),
  })
  const wantedQ = useQuery<{ wanted: WantedRow[] }>({
    queryKey: ['wanted'],
    queryFn: () => api.get('/api/v1/wanted/'),
  })
  const [activity, setActivity] = useState<SSEMessage[]>([])
  const { connected } = useEvents((m) => setActivity((prev) => [m, ...prev].slice(0, 10)))

  const titles = titlesQ.data?.titles ?? []
  const wanted = wantedQ.data?.wanted ?? []

  return (
    <div className="space-y-6 max-w-6xl">
      <header className="flex items-baseline justify-between">
        <h1 className="text-3xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
          dashboard
        </h1>
        <span className="text-xs text-[color:var(--fg-2)]">
          events: <span className={connected ? 'text-[color:var(--success)]' : 'text-[color:var(--fg-2)]'}>
            {connected ? 'connected' : 'offline'}
          </span>
        </span>
      </header>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Stat label="titles" value={titles.length} />
        <Stat label="wanted" value={wanted.length} hint="no commentary yet" />
        <Stat
          label="top candidate"
          value={wanted[0]?.candidates?.length ?? 0}
          hint={wanted[0]?.title_id ?? 'no wanted titles'}
        />
      </div>

      <section>
        <h2 className="text-xl font-semibold mb-3">recent activity</h2>
        <div className="rounded-xl border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-4">
          {activity.length === 0 ? (
            <p className="text-sm text-[color:var(--fg-2)]">no events yet…</p>
          ) : (
            <ul className="space-y-1 mono text-sm">
              {activity.map((m, i) => (
                <li key={i}>
                  <span className="text-[color:var(--accent)]">{m.event}</span>{' '}
                  <span className="text-[color:var(--fg-2)]">{m.data}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>
    </div>
  )
}
