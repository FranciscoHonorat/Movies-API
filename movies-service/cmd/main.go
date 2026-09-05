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

	// 2. Conexão com MongoDB
	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Erro ao conectar no MongoDB: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("Erro ao pingar MongoDB: %v", err)
	}

	collection := client.Database("moviesDB").Collection("movies")

	if err := seed.Seed(ctx, collection, "movies.json"); err != nil {
		log.Printf("Aviso ao executar o seed: %v", err)
	}

	repo := mongodb.NewMovieRepository(collection)
	svc := service.NewMovieService(repo)

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
			return
		}

		movie, err := entity.NewMovieEntity(id, payload.Title, payload.Year)
		if err != nil {
			log.Printf("Erro ao validar filme recebido via fila: %v", err)
			return
		}

		if _, err := svc.CreateMovie(ctx, movie); err != nil {
			log.Printf("Erro ao processar filme via consumidor: %v", err)
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
