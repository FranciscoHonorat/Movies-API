package output

import (
	"context"
	"movies-service/internal/core/domain/entity"
)

type JobStatus struct {
	Status  string
	MovieID int32
	Error   string
}

type MovieJobStatus struct {
	Status string
	Movie  *entity.MovieEntity
	Error  string
}

type MovieJobRepository interface {
	SaveCompleted(ctx context.Context, correlationID string, movieID int32) error
	SaveFailed(ctx context.Context, correlationID string, errMsg string) error
	GetStatus(ctx context.Context, correlationID string) (*JobStatus, error)
}
