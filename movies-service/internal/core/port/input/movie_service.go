package input

import (
	"context"
	"movies-service/internal/core/domain/entity"
	"movies-service/internal/core/port/output"
)

type MovieService interface {
	GetMovieByID(ctx context.Context, id int32) (*entity.MovieEntity, error)
	ListMovies(ctx context.Context, filters output.Listfilters, pagination output.Pagination, sorting output.Sorting) ([]*entity.MovieEntity, error)
	CountMovies(ctx context.Context, filters output.Listfilters) (int32, error)
	CreateMovie(ctx context.Context, movie *entity.MovieEntity) (*entity.MovieEntity, error)
	DeleteMovie(ctx context.Context, id int32) error
	// NextMovieID returns a new, previously unused movie ID.
	NextMovieID(ctx context.Context) (int32, error)
	// RecordJobCompleted marks an async movie-creation job (identified by the
	// correlation ID the gateway handed the client) as successfully finished.
	RecordJobCompleted(ctx context.Context, correlationID string, movieID int32) error
	// RecordJobFailed marks an async movie-creation job as failed.
	RecordJobFailed(ctx context.Context, correlationID string, errMsg string) error
	// GetJobStatus resolves the current state of an async movie-creation job,
	// fetching the created movie when the job has completed.
	GetJobStatus(ctx context.Context, correlationID string) (*output.MovieJobStatus, error)
}
