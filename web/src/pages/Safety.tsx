import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, APIError } from '../api/client'

const EXAMPLES = [
  { name: 'Strict commentary replace', expression: 'classifier_confidence >= 0.85' },
  {
    name: 'Must add ≥1 audio track',
    expression: 'audio_track_count > original_audio_track_count',
  },
  {
    name: 'Reject bitrate regression',
    expression: 'video_bitrate_mbps >= original_video_bitrate_mbps * 0.8',
  },
  {
    name: 'Require labeled commentary track',
    expression: 'classifier_commentary_track_count >= 1',
  },
  { name: 'Container must be MKV', expression: "container == 'mkv'" },
  { name: 'Magic-byte check', expression: 'file_magic_matches_extension == true' },
]

const ACTIONS = ['block_replace', 'block_import', 'warn', 'log_only'] as const

type Rule = {
  ID: string
  Name: string
  Expression: string
  Action: typeof ACTIONS[number]
  Enabled: boolean
}

export function Safety() {
  const qc = useQueryClient()
  const [id, setId] = useState('default')
  const [name, setName] = useState('default')
  const [expr, setExpr] = useState('classifier_confidence >= 0.85')
  const [action, setAction] = useState<typeof ACTIONS[number]>('block_replace')
  const [feedback, setFeedback] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null)

  const list = useQuery<{ rules: Rule[] | null }>({
    queryKey: ['safety-rules'],
    queryFn: () => api.get('/api/v1/safety/rules'),
  })

  const validate = useMutation({
    mutationFn: (expression: string) =>
      api.post('/api/v1/safety/rules/validate', { expression }),
    onSuccess: () => setFeedback({ kind: 'ok', text: 'compiles cleanly' }),
    onError: (e: unknown) =>
      setFeedback({
        kind: 'err',
        text: e instanceof APIError ? e.message : String(e),
      }),
  })

  const save = useMutation({
    mutationFn: () =>
      api.post('/api/v1/safety/rules', {
        ID: id, Name: name, Expression: expr, Action: action, Enabled: true,
      }),
    onSuccess: () => {
      setFeedback({ kind: 'ok', text: 'saved' })
      qc.invalidateQueries({ queryKey: ['safety-rules'] })
    },
    onError: (e: unknown) =>
      setFeedback({
        kind: 'err',
        text: e instanceof APIError ? e.message : String(e),
      }),
  })

  const del = useMutation({
    mutationFn: (delID: string) =>
      api.delete<void>(`/api/v1/safety/rules/${encodeURIComponent(delID)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['safety-rules'] }),
  })

  const rules = list.data?.rules ?? []

  return (
    <div className="space-y-6 max-w-5xl">
      <h1 className="text-3xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
        safety rules
      </h1>
      <p className="text-sm text-[color:var(--fg-1)] max-w-2xl">
        Rules are authored as <code className="mono text-[color:var(--accent)]">CEL</code>{' '}
        expressions evaluated against the fact bundle. Expressions must return{' '}
        <code className="mono">bool</code>.
      </p>

      {rules.length > 0 && (
        <section>
          <h2 className="text-sm uppercase tracking-widest text-[color:var(--fg-2)] mb-2">
            registered rules
          </h2>
          <ul className="space-y-2">
            {rules.map((r) => (
              <li
                key={r.ID}
                className="rounded-lg border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-3 flex items-start gap-3"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-baseline gap-2">
                    <span className="font-semibold">{r.Name}</span>
                    <span className="mono text-xs text-[color:var(--fg-2)]">{r.ID}</span>
                    <span className="mono text-xs px-2 py-0.5 rounded-md bg-[color:var(--bg-2)] text-[color:var(--fg-1)]">
                      {r.Action}
                    </span>
                  </div>
                  <div className="mono text-xs text-[color:var(--fg-1)] mt-1 break-all">
                    {r.Expression}
                  </div>
                </div>
                <button
                  onClick={() => del.mutate(r.ID)}
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
          examples — click to load
        </h2>
        <div className="flex flex-wrap gap-2">
          {EXAMPLES.map((ex) => (
            <button
              key={ex.name}
              onClick={() => {
                setExpr(ex.expression)
                setFeedback(null)
              }}
              className="text-xs rounded-full border border-[color:var(--bg-2)] px-3 py-1 hover:border-[color:var(--accent)]"
            >
              {ex.name}
            </button>
          ))}
        </div>
      </section>

      <section className="space-y-3">
        <div className="grid grid-cols-3 gap-3">
          <div>
            <label className="block text-xs text-[color:var(--fg-2)] mb-1">id</label>
            <input
              value={id}
              onChange={(e) => setId(e.target.value)}
              className="mono w-full rounded-md bg-[color:var(--bg-0)] border border-[color:var(--bg-2)] px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-[color:var(--fg-2)] mb-1">name</label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full rounded-md bg-[color:var(--bg-0)] border border-[color:var(--bg-2)] px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-[color:var(--fg-2)] mb-1">action</label>
            <select
              value={action}
              onChange={(e) => setAction(e.target.value as typeof ACTIONS[number])}
              className="mono w-full rounded-md bg-[color:var(--bg-0)] border border-[color:var(--bg-2)] px-3 py-2 text-sm"
            >
              {ACTIONS.map((a) => (
                <option key={a} value={a}>{a}</option>
              ))}
            </select>
          </div>
        </div>
        <textarea
          value={expr}
          onChange={(e) => {
            setExpr(e.target.value)
            setFeedback(null)
          }}
          rows={6}
          className="mono w-full rounded-md bg-[color:var(--bg-0)] border border-[color:var(--bg-2)] p-3 text-sm focus:border-[color:var(--accent)] focus:outline-none"
        />
        <div className="flex items-center gap-3">
          <button
            onClick={() => validate.mutate(expr)}
            disabled={validate.isPending || !expr.trim()}
            className="rounded-md border border-[color:var(--bg-2)] px-4 py-2 text-sm hover:border-[color:var(--accent)]"
          >
            {validate.isPending ? 'checking…' : 'validate'}
          </button>
          <button
            onClick={() => save.mutate()}
            disabled={save.isPending || !expr.trim() || !id.trim()}
            className="rounded-md bg-[color:var(--accent)] text-[color:var(--bg-0)] font-semibold px-4 py-2 disabled:opacity-40 hover:bg-[color:var(--accent-dim)]"
          >
            {save.isPending ? 'saving…' : 'save rule'}
          </button>
          {feedback && (
            <span
              className={
                'text-sm ' +
                (feedback.kind === 'ok'
                  ? 'text-[color:var(--success)]'
                  : 'text-[color:var(--error)]')
              }
            >
              {feedback.text}
            </span>
          )}
        </div>
      </section>

      <section className="rounded-xl border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-4 mono text-xs text-[color:var(--fg-1)]">
        <div className="text-[color:var(--accent)] mb-2">available fields</div>
        <div className="grid grid-cols-2 gap-x-6 gap-y-1">
          <span>classifier_confidence: double</span>
          <span>classifier_commentary_track_count: int</span>
          <span>audio_track_count: int</span>
          <span>original_audio_track_count: int</span>
          <span>video_bitrate_mbps: double</span>
          <span>original_video_bitrate_mbps: double</span>
          <span>container: string</span>
          <span>file_magic_matches_extension: bool</span>
          <span>file_size_bytes: int</span>
          <span>release_title: string</span>
          <span>release_group: string</span>
          <span>indexer: string</span>
          <span>seeders: int</span>
          <span>duration_seconds: double</span>
        </div>
      </section>
    </div>
  )
}
