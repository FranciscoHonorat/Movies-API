package main

import (
	"context"
	"encoding/json"
	"log"
	grpcserver "movies-service/internal/adapters/grpc-server"
	"movies-service/internal/adapters/mongodb"
	"movies-service/internal/adapters/rabbitmq"
	"movies-service/internal/adapters/seed"
	"movies-service/internal/core/domain/entity"
	"movies-service/internal/core/service"
	"net"
	"os"

	"github.com/FranciscoHonorat/movies/proto"
	"github.com/FranciscoHonorat/movies/shared"
	"github.com/rabbitmq/amqp091-go"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/grpc"
)

func main() {
	ctx := context.Background()

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	rabbitmqURI := os.Getenv("RABBITMQ_URI")
	if rabbitmqURI == "" {
		rabbitmqURI = "amqp://guest:guest@localhost:5672/"
	}

	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Erro ao conectar no MongoDB: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("Erro ao pingar MongoDB: %v", err)
	}

	collection := client.Database("moviesDB").Collection("movies")
	jobsCollection := client.Database("moviesDB").Collection("movie_jobs")

	if err := seed.Seed(ctx, collection, "movies.json"); err != nil {
		log.Printf("Aviso ao executar o seed: %v", err)
	}

	if err := mongodb.EnsureIndexes(ctx, collection); err != nil {
		log.Printf("Aviso ao criar índices: %v", err)
	}

	repo := mongodb.NewMovieRepository(collection)
	jobs := mongodb.NewMovieJobRepository(jobsCollection)
	svc := service.NewMovieService(repo, jobs)

	rabbitmqConsumer, err := rabbitmq.NewConsumer(rabbitmqURI, "movies_queue")
	if err != nil {
		log.Fatalf("Erro ao criar consumidor RabbitMQ: %v", err)
	}

	go rabbitmqConsumer.Consume(func(msg amqp091.Delivery) {
		var payload shared.MoviePublisherMessage
		if err := json.Unmarshal(msg.Body, &payload); err != nil {
			log.Printf("Erro ao desserializar mensagem: %v", err)
			return
		}

		id, err := svc.NextMovieID(ctx)
		if err != nil {
			log.Printf("Erro ao gerar ID do filme: %v", err)
			recordJobFailure(ctx, svc, payload.CorrelationID, err)
			return
		}

		movie, err := entity.NewMovieEntity(id, payload.Title, payload.Year)
		if err != nil {
			log.Printf("Erro ao validar filme recebido via fila: %v", err)
			recordJobFailure(ctx, svc, payload.CorrelationID, err)
			return
		}

		createdMovie, err := svc.CreateMovie(ctx, movie)
		if err != nil {
			log.Printf("Erro ao processar filme via consumidor: %v", err)
			recordJobFailure(ctx, svc, payload.CorrelationID, err)
			return
		}

		if err := svc.RecordJobCompleted(ctx, payload.CorrelationID, createdMovie.GetID()); err != nil {
			log.Printf("Erro ao registrar conclusão do job %q: %v", payload.CorrelationID, err)
		}
	})

	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}

	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("Erro ao abrir porta TCP: %v", err)
	}

	grpcSrv := grpc.NewServer()
	proto.RegisterMovieServiceServer(grpcSrv, grpcserver.NewServer(svc))

	log.Printf("Servidor gRPC rodando na porta :%s...", grpcPort)
	if err := grpcSrv.Serve(lis); err != nil {
		log.Fatalf("Erro ao subir servidor gRPC: %v", err)
	}
}

func recordJobFailure(ctx context.Context, svc *service.MovieService, correlationID string, cause error) {
	if err := svc.RecordJobFailed(ctx, correlationID, cause.Error()); err != nil {
		log.Printf("Erro ao registrar falha do job %q: %v", correlationID, err)
	}
}
