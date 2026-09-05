package valueobjects

import (
	errD "movies-service/internal/core/domain/err-d"
	"strings"
)

const maxTitleLength = 300

type MovieTitle struct {
	Title string
}

func NewMovieTitle(title string) (*MovieTitle, error) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" || len(trimmed) > maxTitleLength {
		return nil, errD.ErrTitleNotValid
	}

	return &MovieTitle{Title: title}, nil
}

func (m *MovieTitle) Equals(other *MovieTitle) bool {
	if other == nil {
		return false
	}
	return m.Title == other.Title
}

func (m *MovieTitle) ZeroValue() bool {
	return m.Title == ""
}
