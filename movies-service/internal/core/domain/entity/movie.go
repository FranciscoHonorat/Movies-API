package entity

import (
	errD "movies-service/internal/core/domain/err-d"
	"movies-service/internal/core/domain/valueobjects"
	"strings"
)

type MovieEntity struct {
	ID    valueobjects.MovieID
	Title valueobjects.MovieTitle
	Year  valueobjects.MovieYear
}

func NewMovieEntity(id int32, title, year string) (*MovieEntity, error) {
	movieID, err := valueobjects.NewMovieID(id)
	if err != nil {
		return nil, err
	}

	movieTitle, err := valueobjects.NewMovieTitle(title)
	if err != nil {
		return nil, err
	}

	movieYear, err := valueobjects.NewMovieYear(year)
	if err != nil {
		return nil, err
	}

	return &MovieEntity{
		ID:    *movieID,
		Title: *movieTitle,
		Year:  *movieYear,
	}, nil
}

func NewMovieEntityForCreate(title, year string) (*MovieEntity, error) {
	movieTitle, err := valueobjects.NewMovieTitle(title)
	if err != nil {
		return nil, err
	}

	movieYear, err := valueobjects.NewMovieYear(year)
	if err != nil {
		return nil, err
	}

	return &MovieEntity{
		Title: *movieTitle,
		Year:  *movieYear,
	}, nil
}

func (m *MovieEntity) Validate() error {
	if m.ID.ZeroValue() {
		return errD.ErrIDNotValid
	}
	if m.Title.ZeroValue() || strings.TrimSpace(m.Title.Title) == "" {
		return errD.ErrTitleNotValid
	}
	if m.Year.ZeroValue() || strings.TrimSpace(m.Year.Year) == "" {
		return errD.ErrYearNotValid
	}
	return nil
}

func (m *MovieEntity) GetID() int32 {
	return m.ID.ID()
}

func (m *MovieEntity) GetTitle() string {
	return m.Title.Title
}

func (m *MovieEntity) GetYear() string {
	return m.Year.Year
}

func (m *MovieEntity) Equals(other *MovieEntity) bool {
	if other == nil {
		return false
	}
	return m.ID.Equals(&other.ID) && m.Title.Equals(&other.Title) && m.Year.Equals(&other.Year)
}

func (m *MovieEntity) ZeroValue() bool {
	return m.ID.ZeroValue() && m.Title.ZeroValue() && m.Year.ZeroValue()
}
