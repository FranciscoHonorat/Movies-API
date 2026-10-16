import { useEffect, useState } from 'react'
import MovieGrid from './components/MovieGrid'
import AddMovieModal from './components/AddMovieModal'
import { listMovies, createMovie, getMovieStatus, deleteMovie } from './api'
import './App.css'

const LIMIT = 12

export default function App() {
  const [movies, setMovies] = useState([])
  const [page, setPage] = useState(1)
  const [hasNextPage, setHasNextPage] = useState(false)
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState('title')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  const [modalOpen, setModalOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState(null)

  useEffect(() => {
    const timeout = setTimeout(() => {
      setPage(1)
      fetchMovies({ title: search, page: 1, sort })
    }, 300)
    return () => clearTimeout(timeout)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search, sort])

  useEffect(() => {
    fetchMovies({ title: search, page, sort })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page])

  async function fetchMovies({ title, page, sort }) {
    setLoading(true)
    setError(null)
    try {
      const res = await listMovies({ title, page, limit: LIMIT, sort })
      const data = res.data || []
      setMovies(data)
      setHasNextPage(data.length === LIMIT)
    } catch (err) {
      setError(err.message)
      setMovies([])
    } finally {
      setLoading(false)
    }
  }

  function refresh() {
    fetchMovies({ title: search, page, sort })
  }

  async function handleAddMovie({ title, year }) {
    setSubmitting(true)
    setSubmitError(null)
    try {
      const { correlation_id } = await createMovie({ title, year })
      await pollStatus(correlation_id)
      setModalOpen(false)
      setPage(1)
      fetchMovies({ title: search, page: 1, sort })
    } catch (err) {
      setSubmitError(err.message)
    } finally {
      setSubmitting(false)
    }
  }

  async function pollStatus(correlationId, attempts = 20) {
    for (let i = 0; i < attempts; i++) {
      const res = await getMovieStatus(correlationId)
      if (res?.status === 'failed') {
        throw new Error(res.error || 'Falha ao criar o filme')
      }
      if (res && res.id) {
        return res
      }
      await new Promise((resolve) => setTimeout(resolve, 400))
    }
    throw new Error('Tempo esgotado aguardando a criação do filme')
  }

  async function handleDelete(movie) {
    if (!window.confirm(`Excluir "${movie.title}"?`)) return
    try {
      await deleteMovie(movie.id)
      refresh()
    } catch (err) {
      alert(err.message)
    }
  }

  return (
    <div className="app">
      <header className="topbar">
        <h1 className="logo">🎬 Movies</h1>

        <div className="search-wrapper">
          <input
            className="search-input"
            placeholder="Buscar por título..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>

        <div className="topbar-actions">
          <select value={sort} onChange={(e) => setSort(e.target.value)}>
            <option value="title">Ordenar por título</option>
            <option value="year">Ordenar por ano</option>
          </select>
          <button className="btn-primary" onClick={() => setModalOpen(true)}>
            + Adicionar
          </button>
        </div>
      </header>

      <main className="content">
        <h2 className="section-title">Filmes</h2>

        {error && <p className="error-banner">{error}</p>}

        {loading ? (
          <p className="empty-state">Carregando...</p>
        ) : (
          <MovieGrid movies={movies} onDelete={handleDelete} />
        )}

        <div className="pagination">
          <button disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            Anterior
          </button>
          <span>Página {page}</span>
          <button disabled={!hasNextPage} onClick={() => setPage((p) => p + 1)}>
            Próxima
          </button>
        </div>
      </main>

      {modalOpen && (
        <AddMovieModal
          onClose={() => {
            setModalOpen(false)
            setSubmitError(null)
          }}
          onSubmit={handleAddMovie}
          submitting={submitting}
          error={submitError}
        />
      )}
    </div>
  )
}
