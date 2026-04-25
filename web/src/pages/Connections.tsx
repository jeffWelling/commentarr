import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'

type Indexer = { name: string; kind: string; base_url: string; enabled: boolean }
type DownloadClient = { name: string; kind: string; base_url: string; enabled: boolean }

function Card({
  name,
  kind,
  baseURL,
  enabled,
}: {
  name: string
  kind: string
  baseURL: string
  enabled: boolean
}) {
  return (
    <div className="rounded-xl border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-5">
      <div className="flex items-baseline justify-between">
        <h3 className="text-lg font-semibold">{name}</h3>
        <span
          className={
            'mono text-xs px-2 py-0.5 rounded-md ' +
            (enabled
              ? 'bg-[color:var(--success)]/15 text-[color:var(--success)]'
              : 'bg-[color:var(--bg-2)] text-[color:var(--fg-2)]')
          }
        >
          {enabled ? 'enabled' : 'disabled'}
        </span>
      </div>
      <div className="mt-1 text-xs uppercase tracking-widest text-[color:var(--fg-2)]">{kind}</div>
      <div className="mt-3 mono text-sm break-all">{baseURL}</div>
    </div>
  )
}

function Section({ title, hint, empty, children }: {
  title: string
  hint: string
  empty: string
  children: React.ReactNode
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
}) {
  return (
    <section>
      <header className="mb-3 flex items-baseline justify-between">
        <h2 className="text-xl font-semibold">{title}</h2>
        <span className="text-xs text-[color:var(--fg-2)]">{hint}</span>
      </header>
      {children ?? <p className="text-sm text-[color:var(--fg-2)]">{empty}</p>}
    </section>
  )
}

export function Connections() {
  const idxQ = useQuery<{ indexers: Indexer[] | null }>({
    queryKey: ['indexers'],
    queryFn: () => api.get('/api/v1/indexers/'),
  })
  const dlQ = useQuery<{ clients: DownloadClient[] | null }>({
    queryKey: ['download-clients'],
    queryFn: () => api.get('/api/v1/download-clients/'),
  })

  const indexers = idxQ.data?.indexers ?? []
  const clients = dlQ.data?.clients ?? []

  return (
    <div className="space-y-8 max-w-5xl">
      <header>
        <h1 className="text-3xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
          connections
        </h1>
        <p className="mt-1 text-sm text-[color:var(--fg-2)]">
          process-level connection cards. configure via <code className="mono">-prowlarr-url</code> /{' '}
          <code className="mono">-qbit-url</code> flags or the chart's{' '}
          <code className="mono">connections.*</code> values.
        </p>
      </header>

      <Section
        title="indexers"
        hint={`${indexers.length} configured`}
        empty="no indexers configured — pass -prowlarr-url to commentarr serve"
      >
        {indexers.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {indexers.map((i) => (
              <Card key={i.name} {...i} baseURL={i.base_url} />
            ))}
          </div>
        )}
      </Section>

      <Section
        title="download clients"
        hint={`${clients.length} configured`}
        empty="no download clients configured — pass -qbit-url to commentarr serve"
      >
        {clients.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {clients.map((c) => (
              <Card key={c.name} {...c} baseURL={c.base_url} />
            ))}
          </div>
        )}
      </Section>
    </div>
  )
}
