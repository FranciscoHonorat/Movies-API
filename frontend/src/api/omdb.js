const API_KEY = import.meta.env.VITE_OMDB_API_KEY
const OMDB_URL = 'https://www.omdbapi.com/'

const cache = new Map()

function stripYear(title) {
  return title.replace(/\s*\(\d{4}\)\s*$/, '').trim()
}

export async function findPosterUrl(title, year) {
  if (!API_KEY) return null

  const cacheKey = `${title}::${year}`
  if (cache.has(cacheKey)) return cache.get(cacheKey)

  const params = new URLSearchParams({
    apikey: API_KEY,
    t: stripYear(title),
  })
  if (year) params.set('y', year)

  try {
    const res = await fetch(`${OMDB_URL}?${params.toString()}`)
    if (!res.ok) throw new Error('OMDb request failed')

    const body = await res.json()
    const url = body.Poster && body.Poster !== 'N/A' ? body.Poster : null

    cache.set(cacheKey, url)
    return url
  } catch {
    cache.set(cacheKey, null)
    return null
  }
}
