package mongodb

import (
	"context"
	"errors"
	"movies-service/internal/core/port/output"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type jobDocument struct {
	CorrelationID string `bson:"_id"`
	Status        string `bson:"status"`
	MovieID       int32  `bson:"movie_id,omitempty"`
	Error         string `bson:"error,omitempty"`
}

type MovieJob struct {
	collection *mongo.Collection
}

func NewMovieJobRepository(collection *mongo.Collection) output.MovieJobRepository {
	return &MovieJob{collection: collection}
}

func (r *MovieJob) SaveCompleted(ctx context.Context, correlationID string, movieID int32) error {
	_, err := r.collection.UpdateByID(ctx, correlationID, bson.M{
		"$set": bson.M{"status": "completed", "movie_id": movieID},
	}, options.UpdateOne().SetUpsert(true))
	return err
}

func (r *MovieJob) SaveFailed(ctx context.Context, correlationID string, errMsg string) error {
	_, err := r.collection.UpdateByID(ctx, correlationID, bson.M{
		"$set": bson.M{"status": "failed", "error": errMsg},
	}, options.UpdateOne().SetUpsert(true))
	return err
}

func (r *MovieJob) GetStatus(ctx context.Context, correlationID string) (*output.JobStatus, error) {
	var doc jobDocument
	err := r.collection.FindOne(ctx, bson.M{"_id": correlationID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &output.JobStatus{Status: "pending"}, nil
	}
	if err != nil {
		return nil, err
	}

	return &output.JobStatus{Status: doc.Status, MovieID: doc.MovieID, Error: doc.Error}, nil
}
