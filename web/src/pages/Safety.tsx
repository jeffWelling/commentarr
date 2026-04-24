import { useMutation } from '@tanstack/react-query'
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

export function Safety() {
  const [expr, setExpr] = useState('classifier_confidence >= 0.85')
  const [feedback, setFeedback] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null)

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
            className="rounded-md bg-[color:var(--accent)] text-[color:var(--bg-0)] font-semibold px-4 py-2 disabled:opacity-40 hover:bg-[color:var(--accent-dim)]"
          >
            {validate.isPending ? 'checking…' : 'validate'}
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
