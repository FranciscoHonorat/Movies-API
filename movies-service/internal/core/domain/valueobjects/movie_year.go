package valueobjects

import (
	errD "movies-service/internal/core/domain/err-d"
	"strconv"
	"time"
)

const minYear = 1888

type MovieYear struct {
	Year string
}

func NewMovieYear(year string) (*MovieYear, error) {
	yearInt, err := strconv.Atoi(year)
	if err != nil {
		return nil, errD.ErrYearNotValid
	}

	maxYear := time.Now().Year() + 2
	if yearInt < minYear || yearInt > maxYear {
		return nil, errD.ErrYearNotValid
	}

	return &MovieYear{Year: year}, nil
}

func (m *MovieYear) Equals(other *MovieYear) bool {
	if other == nil {
		return false
	}
	return m.Year == other.Year
}

func (m *MovieYear) ZeroValue() bool {
	return m.Year == ""
}
