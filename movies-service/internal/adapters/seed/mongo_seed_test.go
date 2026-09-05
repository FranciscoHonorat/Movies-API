package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"movies-service/internal/adapters/seed"
)

func TestSeed(t *testing.T) {
	t.Run("Happy path: should seed the collection when it is empty", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017").SetServerSelectionTimeout(2 * time.Second))
		assert.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")

		err = collection.Drop(context.Background())
		assert.NoError(t, err)

		err = seed.Seed(context.Background(), collection, "testdata/movies.json")
		assert.NoError(t, err)

		count, err := collection.CountDocuments(context.Background(), bson.D{})
		assert.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})

	t.Run("Sad path: should not seed the collection when it is not empty", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017").SetServerSelectionTimeout(2 * time.Second))
		assert.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")

		err = collection.Drop(context.Background())
		assert.NoError(t, err)

		_, err = collection.InsertOne(context.Background(), bson.D{{Key: "title", Value: "Existing Movie"}})
		assert.NoError(t, err)

		err = seed.Seed(context.Background(), collection, "testdata/movies.json")
		assert.NoError(t, err)

		count, err := collection.CountDocuments(context.Background(), bson.D{})
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})
}
