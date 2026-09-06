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
	jobs output.MovieJobRepository
}

func NewMovieService(repo output.MovieRepository, jobs output.MovieJobRepository) *MovieService {
	return &MovieService{repo: repo, jobs: jobs}
}

func (s *MovieService) GetMovieByID(ctx context.Context, id int32) (*entity.MovieEntity, error) {
	if id <= 0 {
		return nil, errD.ErrIDNotValid
	}
	return s.repo.GetMovieByID(ctx, id)
}

func (s *MovieService) ListMovies(ctx context.Context, filters output.Listfilters, pagination output.Pagination, sorting output.Sorting) ([]*entity.MovieEntity, error) {
	return s.repo.ListMovies(ctx, filters, pagination, sorting)
}

func (s *MovieService) CountMovies(ctx context.Context, filters output.Listfilters) (int32, error) {
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

func (s *MovieService) DeleteMovie(ctx context.Context, id int32) error {
	if id <= 0 {
		return fmt.Errorf("Invalid ID")
	}
	return s.repo.DeleteMovie(ctx, id)
}

func (s *MovieService) NextMovieID(ctx context.Context) (int32, error) {
	return s.repo.NextID(ctx)
}

func (s *MovieService) RecordJobCompleted(ctx context.Context, correlationID string, movieID int32) error {
	return s.jobs.SaveCompleted(ctx, correlationID, movieID)
}

func (s *MovieService) RecordJobFailed(ctx context.Context, correlationID string, errMsg string) error {
	return s.jobs.SaveFailed(ctx, correlationID, errMsg)
}

func (s *MovieService) GetJobStatus(ctx context.Context, correlationID string) (*output.MovieJobStatus, error) {
	job, err := s.jobs.GetStatus(ctx, correlationID)
	if err != nil {
		return nil, err
	}

	result := &output.MovieJobStatus{Status: job.Status, Error: job.Error}
	if job.Status == "completed" {
		movie, err := s.repo.GetMovieByID(ctx, job.MovieID)
		if err != nil {
			return nil, err
		}
		result.Movie = movie
	}
	return result, nil
}
