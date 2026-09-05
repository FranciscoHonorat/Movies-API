package mongodb_test

import (
	"context"
	"movies-service/internal/adapters/mongodb"
	"movies-service/internal/core/domain/entity"
	"movies-service/internal/core/port/output"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMovieRepository(t *testing.T) {
	validateMovie := func(t *testing.T, movie *entity.MovieEntity, expectedID int, expectedTitle string, expectedYear string) {
		assert.Equal(t, expectedID, movie.GetID())
		assert.Equal(t, expectedTitle, movie.GetTitle())
		assert.Equal(t, expectedYear, movie.GetYear())
	}

	mongoURI := "mongodb://localhost:27017"

	t.Run("Happy Path: GetMovieByID", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(2 * time.Second))
		require.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")
		repo := mongodb.NewMovieRepository(collection)

		testMovie := bson.M{"_id": 1, "title": "Inception", "year": "2010"}
		_, err = collection.InsertOne(context.Background(), testMovie)
		require.NoError(t, err)

		movie, err := repo.GetMovieByID(context.Background(), 1)
		require.NoError(t, err)
		validateMovie(t, movie, 1, "Inception", "2010")

		_, err = collection.DeleteOne(context.Background(), bson.M{"_id": 1})
		require.NoError(t, err)
	})

	t.Run("Happy Path: ListMovies with filters, pagination and sorting", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(2 * time.Second))
		require.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")
		repo := mongodb.NewMovieRepository(collection)

		testMovies := []interface{}{
			bson.M{"_id": 1, "title": "Inception", "year": "2010"},
			bson.M{"_id": 2, "title": "The Dark Knight", "year": "2008"},
			bson.M{"_id": 3, "title": "Interstellar", "year": "2014"},
			bson.M{"_id": 4, "title": "Dunkirk", "year": "2017"},
			bson.M{"_id": 5, "title": "Tenet", "year": "2020"},
		}
		_, err = collection.InsertMany(context.Background(), testMovies)
		require.NoError(t, err)

		filters := output.Listfilters{}
		pagination := output.Pagination{Page: 1, Limit: 2}
		sorting := output.Sorting{SortBy: "year"}

		movies, err := repo.ListMovies(context.Background(), filters, pagination, sorting)
		require.NoError(t, err)
		require.Len(t, movies, 2)

		validateMovie(t, movies[0], 2, "The Dark Knight", "2008")
		validateMovie(t, movies[1], 1, "Inception", "2010")

		_, err = collection.DeleteMany(context.Background(), bson.M{"_id": bson.M{"$in": []int{1, 2, 3, 4, 5}}})
		require.NoError(t, err)
	})

	t.Run("Sad Path: GetMovieByID with non-existent ID", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(2 * time.Second))
		require.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")
		repo := mongodb.NewMovieRepository(collection)

		movie, err := repo.GetMovieByID(context.Background(), 999)
		assert.Nil(t, movie)
		assert.Error(t, err)
	})

	t.Run("Sad Path: ListMovies with no matching filters", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(2 * time.Second))
		require.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")
		repo := mongodb.NewMovieRepository(collection)

		filters := output.Listfilters{Title: "NonExistentMovie"}
		pagination := output.Pagination{Page: 1, Limit: 10}
		sorting := output.Sorting{SortBy: "year"}

		movies, err := repo.ListMovies(context.Background(), filters, pagination, sorting)
		require.NoError(t, err)
		require.Len(t, movies, 0)
	})

	t.Run("Sad Path: ListMovies with invalid regex filter", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(2 * time.Second))
		require.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")
		repo := mongodb.NewMovieRepository(collection)

		filters := output.Listfilters{Title: "["}
		pagination := output.Pagination{Page: 1, Limit: 10}
		sorting := output.Sorting{SortBy: "year"}

		movies, err := repo.ListMovies(context.Background(), filters, pagination, sorting)
		assert.Nil(t, movies)
		assert.Error(t, err)
	})

	t.Run("Happy Path: DeleteMovie via repo", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(2 * time.Second))
		require.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")
		repo := mongodb.NewMovieRepository(collection)

		testMovie := bson.M{"_id": 10, "title": "Inception", "year": "2010"}
		_, err = collection.InsertOne(context.Background(), testMovie)
		require.NoError(t, err)

		err = repo.DeleteMovie(context.Background(), 10)
		require.NoError(t, err)

		movie, err := repo.GetMovieByID(context.Background(), 10)
		assert.Nil(t, movie)
		assert.Error(t, err)
	})

	t.Run("Sad Path: DeleteMovie with non-existent ID", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(2 * time.Second))
		require.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")
		repo := mongodb.NewMovieRepository(collection)

		err = repo.DeleteMovie(context.Background(), 999)
		assert.Error(t, err)
	})

	t.Run("Sad Path: Insert movie with duplicate ID", func(t *testing.T) {
		client, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(2 * time.Second))
		require.NoError(t, err)
		defer client.Disconnect(context.Background())

		collection := client.Database("testdb").Collection("movies")

		testMovie := bson.M{"_id": 1, "title": "Inception", "year": "2010"}
		_, err = collection.InsertOne(context.Background(), testMovie)
		require.NoError(t, err)

		duplicateMovie := bson.M{"_id": 1, "title": "Duplicate Movie", "year": "2020"}
		_, err = collection.InsertOne(context.Background(), duplicateMovie)
		assert.Error(t, err)

		_, err = collection.DeleteOne(context.Background(), bson.M{"_id": 1})
		require.NoError(t, err)
	})

	t.Run("Sad Path: Entity domain validation for missing required fields", func(t *testing.T) {
		invalidMovie, err := entity.NewMovieEntity(2, "", "2020")
		assert.Error(t, err)
		assert.Nil(t, invalidMovie)
	})
}
