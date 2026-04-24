// Minimal typed API client that attaches X-Api-Key from localStorage.

const STORAGE_KEY = 'commentarr.apiKey'

export function setAPIKey(key: string) {
  localStorage.setItem(STORAGE_KEY, key)
}

export function getAPIKey(): string | null {
  return localStorage.getItem(STORAGE_KEY)
}

export function clearAPIKey() {
  localStorage.removeItem(STORAGE_KEY)
}

export class APIError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(`${status}: ${message}`)
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const key = getAPIKey()
  const headers: Record<string, string> = {
    Accept: 'application/json',
  }
  if (key) headers['X-Api-Key'] = key
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const resp = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!resp.ok) {
    const text = await resp.text().catch(() => '')
    throw new APIError(resp.status, text || resp.statusText)
  }
  if (resp.status === 204) return undefined as T
  return (await resp.json()) as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path),
}

/** Validate a stored API key by hitting an authenticated endpoint. */
export async function validateStoredKey(): Promise<boolean> {
  if (!getAPIKey()) return false
  try {
    await api.get('/api/v1/library/titles')
    return true
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return false
    throw e
  }
}
