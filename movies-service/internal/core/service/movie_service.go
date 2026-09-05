package service

import (
	"context"
	"fmt"

	"movies-service/internal/core/domain/entity"
	errD "movies-service/internal/core/domain/err-d"
	"movies-service/internal/core/port/output"
)

type MovieService struct {
	repo output.MovieRepository
}

func NewMovieService(repo output.MovieRepository) *MovieService {
	return &MovieService{repo: repo}
}

func (s *MovieService) GetMovieByID(ctx context.Context, id int) (*entity.MovieEntity, error) {
	if id <= 0 {
		return nil, errD.ErrIDNotValid
	}
	return s.repo.GetMovieByID(ctx, id)
}

func (s *MovieService) ListMovies(ctx context.Context, filters output.Listfilters, pagination output.Pagination, sorting output.Sorting) ([]*entity.MovieEntity, error) {
	return s.repo.ListMovies(ctx, filters, pagination, sorting)
}

func (s *MovieService) CountMovies(ctx context.Context, filters output.Listfilters) (int, error) {
	return s.repo.CountMovies(ctx, filters)
}

func (s *MovieService) CreateMovie(ctx context.Context, movie *entity.MovieEntity) (*entity.MovieEntity, error) {
	if movie == nil {
		return nil, fmt.Errorf("Invalid Movie")
	}
	if err := movie.Validate(); err != nil {
		return nil, fmt.Errorf("Invalid Movie")
	}
	return s.repo.CreateMovie(ctx, movie)
}

func (s *MovieService) DeleteMovie(ctx context.Context, id int) error {
	if id <= 0 {
		return fmt.Errorf("Invalid ID")
	}
	return s.repo.DeleteMovie(ctx, id)
}

func (s *MovieService) NextMovieID(ctx context.Context) (int, error) {
	return s.repo.NextID(ctx)
}
