import { useEffect, useState } from 'react'
import { posterGradient } from '../utils/poster'
import { findPosterUrl } from '../api/omdb'

export default function MovieCard({ movie, onDelete }) {
  const [posterUrl, setPosterUrl] = useState(null)

  useEffect(() => {
    let cancelled = false
    setPosterUrl(null)

    findPosterUrl(movie.title, movie.year).then((url) => {
      if (!cancelled) setPosterUrl(url)
    })

    return () => {
      cancelled = true
    }
  }, [movie.title, movie.year])

  return (
    <div className="movie-card">
      <div
        className="movie-poster"
        style={posterUrl ? undefined : { background: posterGradient(movie.title) }}
      >
        {posterUrl && (
          <img className="movie-poster-img" src={posterUrl} alt={movie.title} loading="lazy" />
        )}
        {!posterUrl && <span className="movie-poster-title">{movie.title}</span>}
        <button
          className="movie-delete"
          title="Excluir filme"
          onClick={() => onDelete(movie)}
        >
          &times;
        </button>
      </div>
      <div className="movie-info">
        <p className="movie-title">{movie.title}</p>
        <p className="movie-year">{movie.year}</p>
      </div>
    </div>
  )
}
