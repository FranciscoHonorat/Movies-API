package mongodb

import (
	"context"
	"errors"
	"movies-service/internal/core/domain/entity"
	errD "movies-service/internal/core/domain/err-d"
	"movies-service/internal/core/port/output"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("movies-service/mongodb")

// withSpan wraps a Mongo call in a span named after the operation, so a
// trace shows where time actually went (e.g. "mongodb.NextID" contending
// on the counter document — see docs/adr/0005-*.md — is a lot easier to
// spot in Jaeger than in a log line). Errors are recorded on the span but
// still returned unchanged to the caller; this package doesn't change
// what any method returns, only what gets reported alongside it.
func withSpan[T any](ctx context.Context, name string, fn func(context.Context) (T, error)) (T, error) {
	ctx, span := tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	result, err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

const idCounterDocID = "movie_id"

type Movie struct {
	collection *mongo.Collection
	counters   *mongo.Collection

	counterOnce sync.Once
	counterErr  error
}

type movieRepository struct {
	Id    int32  `bson:"_id"`
	Title string `bson:"title"`
	Year  string `bson:"year"`
}

func NewMovieRepository(collection *mongo.Collection) output.MovieRepository {
	return &Movie{
		collection: collection,
		counters:   collection.Database().Collection("counters"),
	}
}

// EnsureIndexes creates the indexes ListMovies relies on for filtering and
// sorting. Without them, `Find` with a `title`/`year` sort (the default —
// see api-gateway's ListMovie handler) forces Mongo into a full collection
// scan plus an in-memory sort on every request. Measured against the
// ~28k-document seed dataset: unindexed, GET /movies collapses from
// stable (p99 ~120ms) to a full outage (mean 3.7s, p99 6.9s, MongoDB
// pegged at ~580% CPU) between 60 and 80 req/s — see
// docs/performance/README.md. `CreateMany` is idempotent (creating an
// index that already exists with the same spec is a no-op), so calling
// this on every boot is safe.
func EnsureIndexes(ctx context.Context, collection *mongo.Collection) error {
	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "title", Value: 1}}},
		{Keys: bson.D{{Key: "year", Value: 1}}},
	})
	return err
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

func (m *Movie) GetMovieByID(ctx context.Context, id int32) (*entity.MovieEntity, error) {
	return withSpan(ctx, "mongodb.GetMovieByID", func(ctx context.Context) (*entity.MovieEntity, error) {
		filter := bson.M{"_id": id}

		var doc movieRepository
		err := m.collection.FindOne(ctx, filter).Decode(&doc)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errD.ErrMovieNotFound
		}
		if err != nil {
			return nil, err
		}

		return ToDomain(doc)
	})
}

func (m *Movie) ListMovies(ctx context.Context, filters output.Listfilters, pagination output.Pagination, sorting output.Sorting) ([]*entity.MovieEntity, error) {
	return withSpan(ctx, "mongodb.ListMovies", func(ctx context.Context) ([]*entity.MovieEntity, error) {
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
	})
}

func (m *Movie) CountMovies(ctx context.Context, filters output.Listfilters) (int32, error) {
	return withSpan(ctx, "mongodb.CountMovies", func(ctx context.Context) (int32, error) {
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

		return int32(count), nil
	})
}

func (m *Movie) CreateMovie(ctx context.Context, movie *entity.MovieEntity) (*entity.MovieEntity, error) {
	return withSpan(ctx, "mongodb.CreateMovie", func(ctx context.Context) (*entity.MovieEntity, error) {
		opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
		filter := bson.M{"_id": movie.GetID()}
		update := bson.M{"$set": ToDocument(movie)}

		var updatedDoc movieRepository
		err := m.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&updatedDoc)
		if err != nil {
			return nil, err
		}

		return ToDomain(updatedDoc)
	})
}

func (m *Movie) DeleteMovie(ctx context.Context, id int32) error {
	_, err := withSpan(ctx, "mongodb.DeleteMovie", func(ctx context.Context) (struct{}, error) {
		filter := bson.M{"_id": id}

		result, err := m.collection.DeleteOne(ctx, filter)
		if err != nil {
			return struct{}{}, err
		}
		if result.DeletedCount == 0 {
			return struct{}{}, errD.ErrMovieNotFound
		}
		return struct{}{}, nil
	})
	return err
}

func (m *Movie) NextID(ctx context.Context) (int32, error) {
	return withSpan(ctx, "mongodb.NextID", func(ctx context.Context) (int32, error) {
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
			Seq int32 `bson:"seq"`
		}
		if err := m.counters.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc); err != nil {
			return 0, err
		}
		return doc.Seq, nil
	})
}

func (m *Movie) seedCounterFromExistingMovies(ctx context.Context) error {
	opts := options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}})

	var doc struct {
		ID int32 `bson:"_id"`
	}
	err := m.collection.FindOne(ctx, bson.M{}, opts).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return err
	}

	_, err = m.counters.UpdateOne(ctx,
		bson.M{"_id": idCounterDocID},
		bson.M{"$max": bson.M{"seq": doc.ID}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}
