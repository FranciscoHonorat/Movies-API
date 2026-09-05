package valueobjects_test

import (
	errD "movies-service/internal/core/domain/err-d"
	"movies-service/internal/core/domain/valueobjects"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMovieYear(t *testing.T) {
	validateMovieYear := func(year string, expectedError bool) {
		t.Helper()
		_, err := valueobjects.NewMovieYear(year)
		if expectedError {
			assert.Error(t, err)
			assert.Equal(t, errD.ErrYearNotValid, err)
		} else {
			assert.NoError(t, err)
		}
	}

	t.Run("Valid Year: dentro da faixa", func(t *testing.T) {
		validateMovieYear("2010", false)
	})
	t.Run("Valid Year: limite inferior (1888)", func(t *testing.T) {
		validateMovieYear("1888", false)
	})
	t.Run("Invalid Year: vazio", func(t *testing.T) {
		validateMovieYear("", true)
	})
	t.Run("Invalid Year: não numérico", func(t *testing.T) {
		validateMovieYear("Inception", true)
	})
	t.Run("Invalid Year: anterior a 1888", func(t *testing.T) {
		validateMovieYear("1800", true)
	})
	t.Run("Invalid Year: longe demais no futuro", func(t *testing.T) {
		validateMovieYear("3000", true)
	})
}
