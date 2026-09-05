package valueobjects_test

import (
	"testing"

	errD "movies-service/internal/core/domain/err-d"
	"movies-service/internal/core/domain/valueobjects"

	"github.com/stretchr/testify/assert"
)

func TestNewMovieID(t *testing.T) {
	validateMovieID := func(id int32, expectedError bool) {
		t.Helper()
		_, err := valueobjects.NewMovieID(id)
		if expectedError {
			assert.Error(t, err)
			assert.Equal(t, errD.ErrIDNotValid, err)
		} else {
			assert.NoError(t, err)
		}
	}
	t.Run("Valid ID", func(t *testing.T) {
		validateMovieID(1, false)
	})
	t.Run("Invalid ID", func(t *testing.T) {
		validateMovieID(0, true)
		validateMovieID(-1, true)
	})
}
