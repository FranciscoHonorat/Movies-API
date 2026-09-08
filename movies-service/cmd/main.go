package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	grpcserver "movies-service/internal/adapters/grpc-server"
	"movies-service/internal/adapters/mongodb"
	"movies-service/internal/adapters/rabbitmq"
	"movies-service/internal/adapters/seed"
	"movies-service/internal/core/domain/entity"
	"movies-service/internal/core/service"
	"movies-service/internal/observability"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	hermes "github.com/FranciscoHonorat/hermes-observability/packages/agent-go"
	"github.com/FranciscoHonorat/hermes-observability/packages/agent-go/grpcmetrics"
	"github.com/FranciscoHonorat/movies/proto"
	"github.com/FranciscoHonorat/movies/shared"
	"github.com/rabbitmq/amqp091-go"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

var tracer = otel.Tracer("movies-service/consumer")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.InitTracing(ctx, "movies-service", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			slog.Error("erro ao encerrar tracing", "error", err)
		}
	}()

	hermesAgent := hermes.NewClient() // reads HERMES_* env vars, see docker-compose.yml
	hermesAgent.Start()
	defer hermesAgent.Stop()

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

	consumerWorkers := 4
	if v := os.Getenv("CONSUMER_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			consumerWorkers = n
		} else {
			log.Printf("CONSUMER_WORKERS=%q inválido, usando o padrão (%d)", v, consumerWorkers)
		}
	}

	if err := rabbitmqConsumer.Consume(func(msg amqp091.Delivery) {
		// The message carries the publisher's trace context in its AMQP
		// headers (injected by api-gateway — see
		// api-gateway/internal/adapters/rabbitmq/publisher.go); extracting
		// it here is what makes this span (and everything under it —
		// NextMovieID, CreateMovie, RecordJobCompleted, all the way down
		// to the Mongo calls) show up as part of the *same* trace as the
		// original POST /movies request in Jaeger, instead of a disconnected
		// trace with no visible cause. Based on context.Background(), not
		// the process-lifetime ctx above, so an in-flight message keeps
		// running to completion during a graceful shutdown instead of
		// having its context cancelled out from under it.
		msgCtx := otel.GetTextMapPropagator().Extract(context.Background(), shared.AMQPHeaderCarrier(msg.Headers))
		msgCtx, span := tracer.Start(msgCtx, "movies_queue consume", trace.WithSpanKind(trace.SpanKindConsumer))
		defer span.End()

		var payload shared.MoviePublisherMessage
		if err := json.Unmarshal(msg.Body, &payload); err != nil {
			log.Printf("Erro ao desserializar mensagem: %v", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "falha ao desserializar a mensagem")
			return
		}

		id, err := svc.NextMovieID(msgCtx)
		if err != nil {
			log.Printf("Erro ao gerar ID do filme: %v", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "falha ao gerar o próximo ID")
			recordJobFailure(msgCtx, svc, payload.CorrelationID, err)
			return
		}

		movie, err := entity.NewMovieEntity(id, payload.Title, payload.Year)
		if err != nil {
			log.Printf("Erro ao validar filme recebido via fila: %v", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "validação do filme falhou")
			recordJobFailure(msgCtx, svc, payload.CorrelationID, err)
			return
		}

		createdMovie, err := svc.CreateMovie(msgCtx, movie)
		if err != nil {
			log.Printf("Erro ao processar filme via consumidor: %v", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "CreateMovie falhou")
			recordJobFailure(msgCtx, svc, payload.CorrelationID, err)
			return
		}

		if err := svc.RecordJobCompleted(msgCtx, payload.CorrelationID, createdMovie.GetID()); err != nil {
			log.Printf("Erro ao registrar conclusão do job %q: %v", payload.CorrelationID, err)
			span.RecordError(err)
		}
	}, consumerWorkers); err != nil {
		log.Fatalf("Erro ao iniciar consumidor RabbitMQ: %v", err)
	}
	log.Printf("Consumindo %q com %d worker(s)", "movies_queue", consumerWorkers)

	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}

	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("Erro ao abrir porta TCP: %v", err)
	}

	grpcSrv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(grpcmetrics.UnaryServerInterceptor(hermesAgent)),
	)
	proto.RegisterMovieServiceServer(grpcSrv, grpcserver.NewServer(svc))

	go func() {
		log.Printf("Servidor gRPC rodando na porta :%s...", grpcPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("Erro ao subir servidor gRPC: %v", err)
		}
	}()

	<-ctx.Done()
	slog.Info("sinal de encerramento recebido, desligando")
	grpcSrv.GracefulStop()
}

func recordJobFailure(ctx context.Context, svc *service.MovieService, correlationID string, cause error) {
	if err := svc.RecordJobFailed(ctx, correlationID, cause.Error()); err != nil {
		log.Printf("Erro ao registrar falha do job %q: %v", correlationID, err)
	}
}
