import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '../api/client'

const ALL_EVENTS = [
  'OnSearch', 'OnGrab', 'OnDownload', 'OnImport', 'OnReplace',
  'OnTrash', 'OnTrashExpire', 'OnRestore', 'OnVerifyFail',
  'OnSafetyViolation', 'OnHealthIssue', 'OnTest',
]

export function Webhooks() {
  const [id, setId] = useState('default')
  const [url, setUrl] = useState('')
  const [events, setEvents] = useState<string[]>(['OnImport', 'OnReplace', 'OnTrash'])
  const [enabled, setEnabled] = useState(true)

  const save = useMutation({
    mutationFn: () =>
      api.post('/api/v1/webhooks/', {
        ID: id, Name: id, URL: url, Events: events, Enabled: enabled,
      }),
  })
  const test = useMutation({
    mutationFn: () => api.post('/api/v1/webhooks/test'),
  })

  return (
    <div className="space-y-6 max-w-3xl">
      <h1 className="text-3xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
        webhooks
      </h1>

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
    </div>
  )
}
