import { useEffect, useRef, useState } from 'react'
import { getAPIKey } from './client'

export type SSEMessage = { event: string; data: string }

/** Subscribe to /api/v1/events. Reconnects with backoff. */
export function useEvents(onMessage?: (msg: SSEMessage) => void) {
  const [connected, setConnected] = useState(false)
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    const key = getAPIKey()
    if (!key) return

    let cancelled = false
    let attempt = 0

    const connect = () => {
      if (cancelled) return
      // EventSource can't send custom headers, so we fall back to the
      // ?apikey= query param here.
      const es = new EventSource(`/api/v1/events?apikey=${encodeURIComponent(key)}`)
      esRef.current = es
      es.addEventListener('open', () => {
        attempt = 0
        setConnected(true)
      })
      es.addEventListener('error', () => {
        setConnected(false)
        es.close()
        if (!cancelled) {
          const delay = Math.min(1000 * 2 ** attempt, 30000)
          attempt++
          setTimeout(connect, delay)
        }
      })
      // Listen for every event type; SSE event.type defaults to "message"
      // unless the server sends `event: <kind>` which it does.
      es.addEventListener('message', (e) => {
        onMessage?.({ event: 'message', data: e.data })
      })
      // Specific events from DESIGN.md § 5.11.
      for (const kind of [
        'hello', 'OnSearch', 'OnGrab', 'OnDownload', 'OnImport', 'OnReplace',
        'OnTrash', 'OnTrashExpire', 'OnRestore', 'OnVerifyFail',
        'OnSafetyViolation', 'OnHealthIssue', 'OnTest',
      ]) {
        es.addEventListener(kind, (e: MessageEvent) => {
          onMessage?.({ event: kind, data: e.data })
        })
      }
    }

    connect()
    return () => {
      cancelled = true
      esRef.current?.close()
      setConnected(false)
    }
  }, [onMessage])

  return { connected }
}
