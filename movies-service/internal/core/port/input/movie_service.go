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
}
