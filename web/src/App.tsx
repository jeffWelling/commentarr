import type { ReactNode } from 'react'
import { BrowserRouter, Link, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Login } from './pages/Login'
import { Dashboard } from './pages/Dashboard'
import { Wanted } from './pages/Wanted'
import { Trash } from './pages/Trash'
import { Safety } from './pages/Safety'
import { Webhooks } from './pages/Webhooks'
import { Connections } from './pages/Connections'
import { Downloads } from './pages/Downloads'
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

function NavBar() {
  const loc = useLocation()
  const items = [
    ['/', 'Dashboard'],
    ['/wanted', 'Wanted'],
    ['/connections', 'Connections'],
    ['/downloads', 'Downloads'],
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
            </Link>
          </li>
        ))}
      </ul>
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
          <Route path="/trash" element={<RequireAuth><Shell><Trash /></Shell></RequireAuth>} />
          <Route path="/safety" element={<RequireAuth><Shell><Safety /></Shell></RequireAuth>} />
          <Route path="/webhooks" element={<RequireAuth><Shell><Webhooks /></Shell></RequireAuth>} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
