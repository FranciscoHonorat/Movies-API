package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/FranciscoHonorat/movies/api-gateway/docs"
	"github.com/FranciscoHonorat/movies/api-gateway/internal/adapters/rabbitmq"
	"github.com/FranciscoHonorat/movies/api-gateway/internal/handlers"
	"github.com/FranciscoHonorat/movies/api-gateway/internal/observability"
	"github.com/FranciscoHonorat/movies/api-gateway/internal/resilience"
	"github.com/FranciscoHonorat/movies/proto"
	hermes "github.com/FranciscoHonorat/hermes-observability/packages/agent-go"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	otelgin "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// @title           Movies API
// @version         1.0.0
// @description     API Gateway para gerenciamento de filmes
// @description     Uma API RESTful que fornece operações CRUD para gerenciar uma coleção de filmes,
// @description     com suporte a paginação, filtragem e ordenação.
// @termsOfService  http://swagger.io/terms/
// @contact.name    API Support
// @license.name    MIT
// @license.url     https://opensource.org/licenses/MIT
// @host            localhost:8080
// @BasePath        /api/v1
// @schemes         http https
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.InitTracing(ctx, "api-gateway", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
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

	conn, err := grpc.NewClient(
		os.Getenv("GRPC_SERVER_URL"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		log.Fatal(err)
	}

	client := resilience.NewMovieServiceClient(proto.NewMovieServiceClient(conn))
	newRabbitMQPublisher, err := rabbitmq.NewRabbitMQPublisher(os.Getenv("RABBITMQ_URI"), "movies_queue")
	if err != nil {
		log.Fatal(err)
	}
	publisher := resilience.NewPublisher(newRabbitMQPublisher)
	movieHandler := handlers.NewMovieHandler(client, publisher)

	r := gin.Default()
	r.Use(otelgin.Middleware("api-gateway"))
	r.Use(observability.GinMiddleware())
	r.Use(observability.HermesGinMiddleware(hermesAgent))

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.GET("/health", handlers.HealthHandler)
	r.GET("/metrics", observability.Handler())

	v1 := r.Group("/api/v1")

	v1.GET("/movies/status/:correlationId", movieHandler.GetMovieStatus)
	v1.GET("/movies/:id", movieHandler.GetMovie)
	v1.GET("/movies", movieHandler.ListMovie)
	v1.POST("/movies", movieHandler.CreateMovie)
	v1.DELETE("/movies/:id", movieHandler.DeleteMovie)

	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
	}

	srv := &http.Server{Addr: ":" + httpPort, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Erro ao subir servidor HTTP: %v", err)
		}
	}()

	// Block on ctx (SIGINT/SIGTERM) instead of srv.ListenAndServe()
	// directly: shutting the HTTP server down explicitly, before the
	// deferred shutdownTracing above runs, is what makes that defer
	// actually reachable. r.Run() (used before this change) blocks
	// forever, so on a real SIGTERM the process would die before ever
	// returning from main — the tracer's buffered spans (BatchSpanProcessor
	// flushes periodically, not per-span) would be silently dropped
	// instead of exported on shutdown.
	<-ctx.Done()
	slog.Info("sinal de encerramento recebido, desligando")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("erro ao desligar servidor HTTP", "error", err)
	}
}
