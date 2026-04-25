import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '../api/client'

const ALL_EVENTS = [
  'OnSearch', 'OnGrab', 'OnDownload', 'OnImport', 'OnReplace',
  'OnTrash', 'OnTrashExpire', 'OnRestore', 'OnVerifyFail',
  'OnSafetyViolation', 'OnHealthIssue', 'OnTest',
]

type Subscriber = {
  ID: string
  Name: string
  URL: string
  Events: string[]
  Enabled: boolean
}

export function Webhooks() {
  const qc = useQueryClient()
  const [id, setId] = useState('default')
  const [url, setUrl] = useState('')
  const [events, setEvents] = useState<string[]>(['OnImport', 'OnReplace', 'OnTrash'])
  const [enabled, setEnabled] = useState(true)

  const list = useQuery<{ webhooks: Subscriber[] | null }>({
    queryKey: ['webhooks'],
    queryFn: () => api.get('/api/v1/webhooks/'),
  })

  const save = useMutation({
    mutationFn: () =>
      api.post('/api/v1/webhooks/', {
        ID: id, Name: id, URL: url, Events: events, Enabled: enabled,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhooks'] }),
  })
  const test = useMutation({
    mutationFn: () => api.post('/api/v1/webhooks/test'),
  })
  const del = useMutation({
    mutationFn: (delID: string) => api.delete<void>(`/api/v1/webhooks/${encodeURIComponent(delID)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhooks'] }),
  })

  const items = list.data?.webhooks ?? []

  return (
    <div className="space-y-6 max-w-3xl">
      <h1 className="text-3xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
        webhooks
      </h1>

      {items.length > 0 && (
        <section>
          <h2 className="text-sm uppercase tracking-widest text-[color:var(--fg-2)] mb-2">registered</h2>
          <ul className="space-y-2">
            {items.map((s) => (
              <li
                key={s.ID}
                className="rounded-lg border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-3 flex items-start gap-3"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-baseline gap-2">
                    <span className="font-semibold">{s.Name || s.ID}</span>
                    <span
                      className={
                        'mono text-xs px-2 py-0.5 rounded-md ' +
                        (s.Enabled
                          ? 'bg-[color:var(--success)]/15 text-[color:var(--success)]'
                          : 'bg-[color:var(--bg-2)] text-[color:var(--fg-2)]')
                      }
                    >
                      {s.Enabled ? 'enabled' : 'disabled'}
                    </span>
                  </div>
                  <div className="mono text-xs text-[color:var(--fg-2)] break-all mt-1">{s.URL}</div>
                  <div className="text-xs text-[color:var(--fg-2)] mt-1">
                    events: {s.Events?.join(', ') || '—'}
                  </div>
                </div>
                <button
                  onClick={() => del.mutate(s.ID)}
                  className="text-sm text-[color:var(--error)] hover:underline shrink-0"
                  disabled={del.isPending}
                >
                  delete
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section>
        <h2 className="text-sm uppercase tracking-widest text-[color:var(--fg-2)] mb-2">
          {items.length > 0 ? 'add another' : 'register'}
        </h2>
        <div className="rounded-xl border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-5 space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-[color:var(--fg-1)] mb-1">id</label>
              <input
                value={id}
                onChange={(e) => setId(e.target.value)}
                className="mono w-full rounded-md bg-[color:var(--bg-0)] border border-[color:var(--bg-2)] px-3 py-2 text-sm"
              />
            </div>
            <div>
              <label className="block text-sm text-[color:var(--fg-1)] mb-1">url</label>
              <input
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://hooks.example.com/commentarr"
                className="mono w-full rounded-md bg-[color:var(--bg-0)] border border-[color:var(--bg-2)] px-3 py-2 text-sm"
              />
            </div>
          </div>
          <div>
            <label className="block text-sm text-[color:var(--fg-1)] mb-2">events</label>
            <div className="flex flex-wrap gap-2">
              {ALL_EVENTS.map((e) => (
                <label key={e} className="text-xs flex items-center gap-1">
                  <input
                    type="checkbox"
                    checked={events.includes(e)}
                    onChange={(ev) => {
                      if (ev.target.checked) setEvents([...events, e])
                      else setEvents(events.filter((x) => x !== e))
                    }}
                  />
                  {e}
                </label>
              ))}
            </div>
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            enabled
          </label>
          <div className="flex gap-2">
            <button
              onClick={() => save.mutate()}
              disabled={!url || save.isPending}
              className="rounded-md bg-[color:var(--accent)] text-[color:var(--bg-0)] font-semibold px-4 py-2 disabled:opacity-40 hover:bg-[color:var(--accent-dim)]"
            >
              {save.isPending ? 'saving…' : 'save'}
            </button>
            <button
              onClick={() => test.mutate()}
              disabled={test.isPending}
              className="rounded-md border border-[color:var(--bg-2)] px-4 py-2 text-sm hover:border-[color:var(--accent)]"
            >
              {test.isPending ? 'sending…' : 'send test event'}
            </button>
          </div>
          {save.isSuccess && <div className="text-[color:var(--success)] text-sm">saved</div>}
          {test.isSuccess && <div className="text-[color:var(--success)] text-sm">OnTest dispatched</div>}
        </div>
      </section>
    </div>
  )
}
