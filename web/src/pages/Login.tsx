import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { APIError, setAPIKey, validateStoredKey } from '../api/client'

export function Login() {
  const nav = useNavigate()
  const [key, setKey] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setErr(null)
    setBusy(true)
    setAPIKey(key.trim())
    try {
      const ok = await validateStoredKey()
      if (!ok) {
        setErr('that key was rejected by the server')
        setBusy(false)
        return
      }
      nav('/')
    } catch (e) {
      setErr(e instanceof APIError ? e.message : 'unexpected error')
      setBusy(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center px-6 animate-page">
      <form
        onSubmit={submit}
        className="w-full max-w-md space-y-6 bg-[color:var(--bg-1)] border border-[color:var(--bg-2)] rounded-xl p-8 shadow-2xl"
      >
        <div>
          <h1 className="text-4xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
            commentarr
          </h1>
          <p className="text-sm text-[color:var(--fg-2)] mt-2">
            paste the <code className="mono text-[color:var(--accent)]">X-Api-Key</code> the
            server printed on first run
          </p>
        </div>
        <div>
          <label className="block text-sm text-[color:var(--fg-1)] mb-2">API key</label>
          <input
            type="password"
            autoFocus
            value={key}
            onChange={(e) => setKey(e.target.value)}
            className="mono w-full rounded-md bg-[color:var(--bg-0)] border border-[color:var(--bg-2)] px-3 py-2 text-[color:var(--fg-0)] focus:border-[color:var(--accent)] focus:outline-none"
            placeholder="0c9004cb715f24af…"
          />
        </div>
        {err && <div className="text-sm text-[color:var(--error)]">{err}</div>}
        <button
          type="submit"
          disabled={busy || !key}
          className="w-full rounded-md bg-[color:var(--accent)] text-[color:var(--bg-0)] font-semibold py-2 disabled:opacity-40 hover:bg-[color:var(--accent-dim)] transition-colors"
        >
          {busy ? 'checking…' : 'enter'}
        </button>
      </form>
    </div>
  )
}
