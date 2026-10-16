import MovieCard from './MovieCard'

export default function MovieGrid({ movies, onDelete }) {
  if (movies.length === 0) {
    return <p className="empty-state">Nenhum filme encontrado.</p>
  }

  return (
    <div className="movie-grid">
      {movies.map((movie) => (
        <MovieCard key={movie.id} movie={movie} onDelete={onDelete} />
      ))}
    </div>
  )
}
