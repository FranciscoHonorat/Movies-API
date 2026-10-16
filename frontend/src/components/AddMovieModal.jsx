import { useState } from 'react'

export default function AddMovieModal({ onClose, onSubmit, submitting, error }) {
  const [title, setTitle] = useState('')
  const [year, setYear] = useState('')

  function handleSubmit(e) {
    e.preventDefault()
    onSubmit({ title: title.trim(), year: year.trim() })
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form
        className="modal"
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSubmit}
      >
        <h2>Adicionar filme</h2>

        <label>
          Título
          <input
            autoFocus
            value={title}
            maxLength={300}
            onChange={(e) => setTitle(e.target.value)}
            required
          />
        </label>

        <label>
          Ano
          <input
            value={year}
            onChange={(e) => setYear(e.target.value)}
            required
          />
        </label>

        {error && <p className="modal-error">{error}</p>}

        <div className="modal-actions">
          <button type="button" className="btn-secondary" onClick={onClose} disabled={submitting}>
            Cancelar
          </button>
          <button type="submit" className="btn-primary" disabled={submitting}>
            {submitting ? 'Salvando...' : 'Salvar'}
          </button>
        </div>
      </form>
    </div>
  )
}
