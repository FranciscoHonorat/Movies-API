package entity_test

import (
	"movies-service/internal/core/domain/entity"
	errD "movies-service/internal/core/domain/err-d"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMovieEntity(t *testing.T) {
	t.Run("New Movie Entity", func(t *testing.T) {
		movies := []struct {
			name       string
			id         int32
			title      string
			year       string
			MovieError error
		}{
			{"Movie valid", 1, "Movie One", "2020", nil},
			{"Movie invalid", 2, "Movie two", "2021", nil},
			{"Movie with invalid id", 0, "Movie Three", "2020", errD.ErrIDNotValid},
			{"Movie with invalid title", 4, "", "2019", errD.ErrTitleNotValid},
			{"Movie with invalid year", 5, "Movie Five", "", errD.ErrYearNotValid},
		}

		for _, movie := range movies {
			t.Run(movie.name, func(t *testing.T) {
				_, err := entity.NewMovieEntity(movie.id, movie.title, movie.year)
				require.Equal(t, movie.MovieError, err)
			})
		}
	})
}
