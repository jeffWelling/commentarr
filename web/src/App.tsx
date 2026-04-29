import type { ReactNode } from 'react'
import { BrowserRouter, Link, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { api } from './api/client'
import { Login } from './pages/Login'
import { Dashboard } from './pages/Dashboard'
import { Wanted } from './pages/Wanted'
import { Trash } from './pages/Trash'
import { Safety } from './pages/Safety'
import { Webhooks } from './pages/Webhooks'
import { Connections } from './pages/Connections'
import { Downloads } from './pages/Downloads'
import { Upgrades } from './pages/Upgrades'
import { clearAPIKey, getAPIKey } from './api/client'

const qc = new QueryClient({
  defaultOptions: { queries: { refetchOnWindowFocus: false, retry: false } },
})

function RequireAuth({ children }: { children: ReactNode }) {
  const location = useLocation()
  if (!getAPIKey()) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }
  return <>{children}</>
}

type SystemInfo = {
  version: string
  commit?: string
  built_at?: string
  go_version?: string
  goos?: string
  goarch?: string
  started_at: string
  uptime_secs: number
}

function formatUptime(secs: number): string {
  if (secs < 60) return `${Math.round(secs)}s`
  const m = Math.floor(secs / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ${m % 60}m`
  const d = Math.floor(h / 24)
  return `${d}d ${h % 24}h`
}

function SystemChip() {
  const q = useQuery<SystemInfo>({
    queryKey: ['system'],
    queryFn: () => api.get('/api/v1/system/'),
    refetchInterval: 30_000,
    enabled: !!getAPIKey(),
  })
  if (!q.data) return null
  const short = q.data.commit?.slice(0, 7) ?? ''
  return (
    <span
      className="mono text-xs text-[color:var(--fg-2)]"
      title={[
        `started: ${q.data.started_at}`,
        q.data.commit ? `commit: ${q.data.commit}` : '',
        q.data.go_version ? `go: ${q.data.go_version}` : '',
        q.data.goos && q.data.goarch ? `${q.data.goos}/${q.data.goarch}` : '',
      ].filter(Boolean).join('\n')}
    >
      {q.data.version}
      {short && <span className="opacity-60"> · {short}</span>}
      <span className="opacity-60"> · up {formatUptime(q.data.uptime_secs)}</span>
    </span>
  )
}

function UpgradesBadge() {
  // Polls /api/v1/upgrades for the count only — the UI doesn't need
  // the full payload here. The Upgrades page renders the detail.
  const q = useQuery<{ upgrades: unknown[] }>({
    queryKey: ['upgrades-count'],
    queryFn: () => api.get('/api/v1/upgrades/'),
    refetchInterval: 60_000,
    enabled: !!getAPIKey(),
  })
  const n = q.data?.upgrades?.length ?? 0
  if (n === 0) return null
  return (
    <span className="ml-1 inline-flex items-center justify-center min-w-5 h-5 px-1 rounded-full text-[10px] font-bold bg-[color:var(--accent)] text-[color:var(--bg-0)]">
      {n}
    </span>
  )
}

function NavBar() {
  const loc = useLocation()
  const items = [
    ['/', 'Dashboard'],
    ['/wanted', 'Wanted'],
    ['/connections', 'Connections'],
    ['/downloads', 'Downloads'],
    ['/upgrades', 'Upgrades'],
    ['/trash', 'Trash'],
    ['/safety', 'Safety'],
    ['/webhooks', 'Webhooks'],
  ] as const
  return (
    <nav className="border-b border-[color:var(--bg-2)] px-6 py-4 flex items-center gap-6">
      <span
        className="text-2xl font-bold tracking-tight"
        style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}
      >
        commentarr
      </span>
      <ul className="flex gap-4 text-[color:var(--fg-1)] flex-1">
        {items.map(([path, label]) => (
          <li key={path}>
            <Link
              to={path}
              className={
                loc.pathname === path
                  ? 'text-[color:var(--accent)] underline underline-offset-4 decoration-2'
                  : 'hover:text-[color:var(--fg-0)] transition-colors'
              }
            >
              {label}
              {path === '/upgrades' && <UpgradesBadge />}
            </Link>
          </li>
        ))}
      </ul>
      <SystemChip />
      <button
        onClick={() => {
          clearAPIKey()
          window.location.href = '/login'
        }}
        className="text-sm text-[color:var(--fg-2)] hover:text-[color:var(--error)] transition-colors"
      >
        sign out
      </button>
    </nav>
  )
}

function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen">
      <NavBar />
      <main className="p-6 animate-page">{children}</main>
    </div>
  )
}

export default function App() {
  return (
    <QueryClientProvider client={qc}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<RequireAuth><Shell><Dashboard /></Shell></RequireAuth>} />
          <Route path="/wanted" element={<RequireAuth><Shell><Wanted /></Shell></RequireAuth>} />
          <Route path="/connections" element={<RequireAuth><Shell><Connections /></Shell></RequireAuth>} />
          <Route path="/downloads" element={<RequireAuth><Shell><Downloads /></Shell></RequireAuth>} />
          <Route path="/upgrades" element={<RequireAuth><Shell><Upgrades /></Shell></RequireAuth>} />
          <Route path="/trash" element={<RequireAuth><Shell><Trash /></Shell></RequireAuth>} />
          <Route path="/safety" element={<RequireAuth><Shell><Safety /></Shell></RequireAuth>} />
          <Route path="/webhooks" element={<RequireAuth><Shell><Webhooks /></Shell></RequireAuth>} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
