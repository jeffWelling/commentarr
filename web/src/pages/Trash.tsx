import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '../api/client'

type Item = {
  ID: number
  Library: string
  OriginalPath: string
  TrashPath: string
  MovedAt: string
  Reason: string
}

export function Trash() {
  const [library, setLibrary] = useState('local')
  const qc = useQueryClient()
  const q = useQuery<{ items: Item[] | null }>({
    queryKey: ['trash', library],
    queryFn: () => api.get(`/api/v1/trash/?library=${encodeURIComponent(library)}`),
    enabled: library.length > 0,
  })
  const del = useMutation({
    mutationFn: (id: number) => api.delete<void>(`/api/v1/trash/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['trash'] }),
  })

  const items = q.data?.items ?? []

  return (
    <div className="space-y-6 max-w-5xl">
      <h1 className="text-3xl font-bold" style={{ fontVariationSettings: '"opsz" 144, "wght" 700' }}>
        trash
      </h1>

      <div className="flex items-center gap-3">
        <label className="text-sm text-[color:var(--fg-1)]">library</label>
        <input
          value={library}
          onChange={(e) => setLibrary(e.target.value)}
          className="mono rounded-md bg-[color:var(--bg-0)] border border-[color:var(--bg-2)] px-3 py-1 text-sm focus:border-[color:var(--accent)] focus:outline-none"
        />
      </div>

      {q.isLoading && <p className="text-[color:var(--fg-2)]">loading…</p>}
      {items.length === 0 && !q.isLoading && (
        <p className="text-[color:var(--fg-2)]">nothing in trash</p>
      )}

      <ul className="space-y-2">
        {items.map((it) => (
          <li
            key={it.ID}
            className="rounded-lg border border-[color:var(--bg-2)] bg-[color:var(--bg-1)] p-3 flex items-center gap-3"
          >
            <div className="flex-1">
              <div className="mono text-sm">{it.OriginalPath}</div>
              <div className="text-xs text-[color:var(--fg-2)]">
                moved {new Date(it.MovedAt).toLocaleString()} · {it.Reason}
              </div>
            </div>
            <button
              onClick={() => del.mutate(it.ID)}
              className="text-sm text-[color:var(--error)] hover:underline"
              disabled={del.isPending}
            >
              delete
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
