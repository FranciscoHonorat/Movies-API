package main

import (
	"log"
	"os"

	_ "github.com/FranciscoHonorat/movies/api-gateway/docs"
	"github.com/FranciscoHonorat/movies/api-gateway/internal/adapters/rabbitmq"
	"github.com/FranciscoHonorat/movies/api-gateway/internal/handlers"
	"github.com/FranciscoHonorat/movies/proto"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
	conn, err := grpc.NewClient(os.Getenv("GRPC_SERVER_URL"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}

	client := proto.NewMovieServiceClient(conn)
	newRabbitMQPublisher, err := rabbitmq.NewRabbitMQPublisher(os.Getenv("RABBITMQ_URI"), "movies_queue")
	if err != nil {
		log.Fatal(err)
	}
	movieHandler := handlers.NewMovieHandler(client, newRabbitMQPublisher)

	r := gin.Default()

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.GET("/health", handlers.HealthHandler)

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
	r.Run(":" + httpPort)
}
