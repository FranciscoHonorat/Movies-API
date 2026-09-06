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

// MovieJobStatus is the resolved view of a job, with the movie fetched when completed.
type MovieJobStatus struct {
	Status string
	Movie  *entity.MovieEntity
	Error  string
}

type MovieJobRepository interface {
	SaveCompleted(ctx context.Context, correlationID string, movieID int32) error
	SaveFailed(ctx context.Context, correlationID string, errMsg string) error
	// GetStatus returns JobStatus{Status: "pending"} when no record exists yet,
	// since the job may still be in flight in the queue.
	GetStatus(ctx context.Context, correlationID string) (*JobStatus, error)
}
