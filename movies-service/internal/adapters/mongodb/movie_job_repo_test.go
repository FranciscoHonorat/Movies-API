package mongodb_test

import (
	"context"
	"testing"
	"time"

	"movies-service/internal/adapters/mongodb"
	"movies-service/internal/core/port/output"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMovieJobRepository(t *testing.T) {
	mongoURI := "mongodb://localhost:27017"

	newRepo := func(t *testing.T) (*mongo.Collection, output.MovieJobRepository) {
		t.Helper()
		client, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(2 * time.Second))
		require.NoError(t, err)
		t.Cleanup(func() { client.Disconnect(context.Background()) })

		collection := client.Database("testdb").Collection("movie_jobs")
		return collection, mongodb.NewMovieJobRepository(collection)
	}

	t.Run("Happy Path: SaveCompleted seguido de GetStatus", func(t *testing.T) {
		collection, repo := newRepo(t)
		t.Cleanup(func() {
			collection.DeleteOne(context.Background(), bson.M{"_id": "corr-completed"})
		})

		err := repo.SaveCompleted(context.Background(), "corr-completed", 42)
		require.NoError(t, err)

		status, err := repo.GetStatus(context.Background(), "corr-completed")
		require.NoError(t, err)
		assert.Equal(t, "completed", status.Status)
		assert.Equal(t, int32(42), status.MovieID)
		assert.Empty(t, status.Error)
	})

	t.Run("Happy Path: SaveFailed seguido de GetStatus", func(t *testing.T) {
		collection, repo := newRepo(t)
		t.Cleanup(func() {
			collection.DeleteOne(context.Background(), bson.M{"_id": "corr-failed"})
		})

		err := repo.SaveFailed(context.Background(), "corr-failed", "title not valid")
		require.NoError(t, err)

		status, err := repo.GetStatus(context.Background(), "corr-failed")
		require.NoError(t, err)
		assert.Equal(t, "failed", status.Status)
		assert.Equal(t, "title not valid", status.Error)
	})

	t.Run("Sad Path: GetStatus para correlation_id inexistente devolve pending", func(t *testing.T) {
		_, repo := newRepo(t)

		status, err := repo.GetStatus(context.Background(), "corr-never-existed")
		require.NoError(t, err)
		assert.Equal(t, "pending", status.Status)
		assert.Zero(t, status.MovieID)
		assert.Empty(t, status.Error)
	})

	t.Run("Happy Path: SaveCompleted sobrescreve estado anterior (upsert)", func(t *testing.T) {
		collection, repo := newRepo(t)
		t.Cleanup(func() {
			collection.DeleteOne(context.Background(), bson.M{"_id": "corr-retry"})
		})

		require.NoError(t, repo.SaveFailed(context.Background(), "corr-retry", "erro transitório"))
		require.NoError(t, repo.SaveCompleted(context.Background(), "corr-retry", 7))

		status, err := repo.GetStatus(context.Background(), "corr-retry")
		require.NoError(t, err)
		assert.Equal(t, "completed", status.Status)
		assert.Equal(t, int32(7), status.MovieID)
	})
}
