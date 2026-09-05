package output

import (
	"context"
	"movies-service/internal/core/domain/entity"
)

type MovieRepository interface {
	GetMovieByID(ctx context.Context, id int32) (*entity.MovieEntity, error)
	ListMovies(ctx context.Context, filters Listfilters, pagination Pagination, sorting Sorting) ([]*entity.MovieEntity, error)
	CountMovies(ctx context.Context, filters Listfilters) (int32, error)
	CreateMovie(ctx context.Context, movie *entity.MovieEntity) (*entity.MovieEntity, error)
	DeleteMovie(ctx context.Context, id int32) error
	// NextID returns a new, previously unused movie ID. It is safe for concurrent use.
	NextID(ctx context.Context) (int32, error)
}

type Listfilters struct {
	Title string
	Year  string
}

type Pagination struct {
	Page  int
	Limit int
}

type Sorting struct {
	SortBy string
}
