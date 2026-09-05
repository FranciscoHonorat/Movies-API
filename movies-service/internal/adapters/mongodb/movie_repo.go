package mongodb

import (
	"context"
	"errors"
	"movies-service/internal/core/domain/entity"
	"movies-service/internal/core/port/output"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// idCounterDocID identifies the single document in the "counters" collection
// that tracks the last movie ID handed out by NextID.
const idCounterDocID = "movie_id"

type Movie struct {
	collection *mongo.Collection
	counters   *mongo.Collection

	counterOnce sync.Once
	counterErr  error
}

type movieRepository struct {
	Id    int    `bson:"_id"`
	Title string `bson:"title"`
	Year  string `bson:"year"`
}

func NewMovieRepository(collection *mongo.Collection) output.MovieRepository {
	return &Movie{
		collection: collection,
		counters:   collection.Database().Collection("counters"),
	}
}

func ToDocument(movie *entity.MovieEntity) movieRepository {
	return movieRepository{
		Id:    movie.GetID(),
		Title: movie.GetTitle(),
		Year:  movie.GetYear(),
	}
}

func ToDomain(doc movieRepository) (*entity.MovieEntity, error) {
	return entity.NewMovieEntity(doc.Id, doc.Title, doc.Year)
}

func (m *Movie) GetMovieByID(ctx context.Context, id int) (*entity.MovieEntity, error) {
	filter := bson.M{"_id": id}

	var doc movieRepository
	err := m.collection.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		return nil, err
	}

	return ToDomain(doc)
}

func (m *Movie) ListMovies(ctx context.Context, filters output.Listfilters, pagination output.Pagination, sorting output.Sorting) ([]*entity.MovieEntity, error) {
	filter := bson.M{}
	if filters.Title != "" {
		filter["title"] = bson.M{"$regex": filters.Title, "$options": "i"}
	}
	if filters.Year != "" {
		filter["year"] = filters.Year
	}

	findOptions := options.Find()
	if pagination.Limit > 0 {
		findOptions.SetLimit(int64(pagination.Limit))
	}
	if pagination.Page > 0 && pagination.Limit > 0 {
		findOptions.SetSkip(int64((pagination.Page - 1) * pagination.Limit))
	}
	if sorting.SortBy != "" {
		findOptions.SetSort(bson.D{bson.E{Key: sorting.SortBy, Value: 1}})
	}

	cursor, err := m.collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var movies []*entity.MovieEntity
	for cursor.Next(ctx) {
		var doc movieRepository
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		movie, err := ToDomain(doc)
		if err != nil {
			return nil, err
		}
		movies = append(movies, movie)
	}

	return movies, nil
}

func (m *Movie) CountMovies(ctx context.Context, filters output.Listfilters) (int, error) {
	filter := bson.M{}
	if filters.Title != "" {
		filter["title"] = bson.M{"$regex": filters.Title, "$options": "i"}
	}
	if filters.Year != "" {
		filter["year"] = filters.Year
	}

	count, err := m.collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	return int(count), nil
}

func (m *Movie) CreateMovie(ctx context.Context, movie *entity.MovieEntity) (*entity.MovieEntity, error) {
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	filter := bson.M{"_id": movie.GetID()}
	update := bson.M{"$set": ToDocument(movie)}

	var updatedDoc movieRepository
	err := m.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&updatedDoc)
	if err != nil {
		return nil, err
	}

	return ToDomain(updatedDoc)
}

func (m *Movie) DeleteMovie(ctx context.Context, id int) error {
	filter := bson.M{"_id": id}

	result, err := m.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// NextID hands out a new, previously unused movie ID via an atomic counter
// document. On first use it seeds the counter from the highest _id already
// present in the movies collection, so it never collides with seeded data.
func (m *Movie) NextID(ctx context.Context) (int, error) {
	m.counterOnce.Do(func() {
		m.counterErr = m.seedCounterFromExistingMovies(ctx)
	})
	if m.counterErr != nil {
		return 0, m.counterErr
	}

	filter := bson.M{"_id": idCounterDocID}
	update := bson.M{"$inc": bson.M{"seq": 1}}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	var doc struct {
		Seq int `bson:"seq"`
	}
	if err := m.counters.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc); err != nil {
		return 0, err
	}
	return doc.Seq, nil
}

func (m *Movie) seedCounterFromExistingMovies(ctx context.Context) error {
	opts := options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}})

	var doc struct {
		ID int `bson:"_id"`
	}
	err := m.collection.FindOne(ctx, bson.M{}, opts).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return err
	}

	// $max creates the field on upsert and only raises it if a lower value is
	// already stored, so this is safe to (re)run even if the counter exists.
	_, err = m.counters.UpdateOne(ctx,
		bson.M{"_id": idCounterDocID},
		bson.M{"$max": bson.M{"seq": doc.ID}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}
