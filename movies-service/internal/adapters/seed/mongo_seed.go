package seed

import (
	"context"
	"encoding/json"
	"os"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type MongoSeed struct {
	ID    int    `bson:"_id" json:"id"`
	Title string `bson:"title" json:"title"`
	Year  string `bson:"year" json:"year"`
}

func Seed(ctx context.Context, collection *mongo.Collection, filepath string) error {
	count, err := collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	data, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	var seeds []MongoSeed
	err = json.Unmarshal(data, &seeds)
	if err != nil {
		return err
	}
	docs := make([]interface{}, len(seeds))
	for i, seed := range seeds {
		docs[i] = seed
	}

	_, err = collection.InsertMany(ctx, docs)
	if err != nil {
		return err
	}

	return nil
}
