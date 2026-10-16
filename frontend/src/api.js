const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080/api/v1'

async function request(path, options) {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })

  if (res.status === 204) return null

  const body = await res.json().catch(() => null)

  if (!res.ok) {
    throw new Error(body?.error || `Request failed with status ${res.status}`)
  }

  return body
}

export function listMovies({ title = '', year = '', page = 1, limit = 12, sort = 'title' } = {}) {
  const params = new URLSearchParams({ page, limit, sort })
  if (title) params.set('title', title)
  if (year) params.set('year', year)
  return request(`/movies?${params.toString()}`)
}

export function createMovie({ title, year }) {
  return request('/movies', {
    method: 'POST',
    body: JSON.stringify({ title, year }),
  })
}

export function getMovieStatus(correlationId) {
  return request(`/movies/status/${correlationId}`)
}

export function deleteMovie(id) {
  return request(`/movies/${id}`, { method: 'DELETE' })
}
