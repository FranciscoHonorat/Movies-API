package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/ports/output"
	"github.com/FranciscoHonorat/movies/proto"
	"github.com/FranciscoHonorat/movies/shared"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MovieHandler struct {
	client    proto.MovieServiceClient
	publisher output.MoviePublisher
}

type CreateMovieStruct struct {
	Title string `json:"title" binding:"required,max=300"`
	Year  string `json:"year" binding:"required"`
}

type ListMovieStruct struct {
	Data  []*proto.Movie `json:"data"`
	Page  int32          `json:"page"`
	Limit int32          `json:"limit"`
}

func NewMovieHandler(client proto.MovieServiceClient, publisher output.MoviePublisher) *MovieHandler {
	return &MovieHandler{client: client, publisher: publisher}
}

// GetMovie godoc
// @Summary      Obter filme por ID
// @Description  Recupera os detalhes completos de um filme específico usando seu ID
// @Tags         Movies
// @Param        id   path      int     true   "ID do filme"  minimum(1)
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}               "Filme encontrado"
// @Failure      400  {object}  map[string]string                    "ID inválido"
// @Failure      404  {object}  map[string]string                    "Filme não encontrado"
// @Failure      500  {object}  map[string]string                    "Erro interno do servidor"
// @Router       /movies/{id} [get]
func (m *MovieHandler) GetMovie(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID is required"})
		return
	}

	idInt, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	movie, err := m.client.GetMovie(c.Request.Context(), &proto.GetMovieRequest{Id: int32(idInt)})

	if err != nil {
		c.JSON(grpcErrorToHTTP(err), gin.H{"error": "Internal server error"})
		return
	}

	c.JSON(http.StatusOK, movie)
}

// ListMovie godoc
// @Summary      Listar filmes
// @Description  Retorna uma lista paginada de filmes com suporte a filtragem, ordenação e busca
// @Tags         Movies
// @Param        title  query     string  false  "Filtro por título (busca parcial)"
// @Param        year   query     string  false  "Filtro por ano de lançamento"
// @Param        page   query     int     false  "Número da página"                    default(1)     minimum(1)
// @Param        limit  query     int     false  "Quantidade de filmes por página"     default(10)    minimum(1)  maximum(100)
// @Param        sort   query     string  false  "Campo para ordenação"                default(title) enums(title,year)
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}               "Lista de filmes recuperada com sucesso"
// @Failure      400  {object}  map[string]string                    "Parâmetros inválidos"
// @Failure      500  {object}  map[string]string                    "Erro interno do servidor"
// @Router       /movies [get]
func (m *MovieHandler) ListMovie(c *gin.Context) {
	title := c.Query("title")
	year := c.Query("year")
	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "10")
	sort := c.DefaultQuery("sort", "title")

	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page (must be >= 1)"})
		return
	}

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt < 1 || limitInt > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit (must be between 1 and 100)"})
		return
	}

	validSorts := map[string]bool{"title": true, "year": true}
	if !validSorts[sort] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sort (must be title, year)"})
		return
	}

	listReq := proto.ListMovieRequest{
		Title:  title,
		Year:   year,
		Page:   int32(pageInt),
		Limit:  int32(limitInt),
		SortBy: sort,
	}

	resp, err := m.client.ListMovie(c.Request.Context(), &listReq)
	if err != nil {
		// Log para ver a mensagem real do erro gRPC nos logs do container
		slog.Error("ListMovie gRPC error", slog.Any("error", err))

		statusHTTP := grpcErrorToHTTP(err)
		c.JSON(statusHTTP, gin.H{"error": err.Error()})
		return
	}

	response := ListMovieStruct{
		Data:  resp.Movie,
		Page:  int32(pageInt),
		Limit: int32(limitInt),
	}

	c.JSON(http.StatusOK, response)
}

// CreateMovie godoc
// @Summary      Criar novo filme
// @Description  Enfileira a criação de um novo filme e devolve um ID de correlação para acompanhar o resultado
// @Tags         Movies
// @Param        body  body      CreateMovieStruct  true   "Dados do novo filme"
// @Accept       json
// @Produce      json
// @Success      202  {object}  map[string]string                    "Criação aceita; consulte o status pelo correlation_id"
// @Failure      400  {object}  map[string]string                    "Dados inválidos ou campos obrigatórios faltando"
// @Failure      500  {object}  map[string]string                    "Erro interno do servidor"
// @Router       /movies [post]
func (m *MovieHandler) CreateMovie(c *gin.Context) {
	var req CreateMovieStruct
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON or missing required fields"})
		return
	}

	correlationID := uuid.NewString()
	err := m.publisher.Publish(c.Request.Context(), shared.MoviePublisherMessage{
		CorrelationID: correlationID,
		Title:         req.Title,
		Year:          req.Year,
	})
	if err != nil {
		slog.Error("CreateMovie error", slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	statusURL := fmt.Sprintf("/api/v1/movies/status/%s", correlationID)
	c.Header("Location", statusURL)
	c.JSON(http.StatusAccepted, gin.H{
		"correlation_id": correlationID,
		"status_url":     statusURL,
	})
}

// GetMovieStatus godoc
// @Summary      Consultar status de criação de filme
// @Description  Consulta o resultado de uma criação assíncrona de filme pelo correlation_id devolvido em POST /movies
// @Tags         Movies
// @Param        correlationId  path      string  true   "Correlation ID devolvido na criação"
// @Accept       json
// @Produce      json
// @Success      200  {object}  proto.Movie                          "Filme criado com sucesso"
// @Success      202  {object}  map[string]string                    "Criação ainda em processamento"
// @Failure      422  {object}  map[string]string                    "Criação falhou"
// @Failure      500  {object}  map[string]string                    "Erro interno do servidor"
// @Router       /movies/status/{correlationId} [get]
func (m *MovieHandler) GetMovieStatus(c *gin.Context) {
	correlationID := c.Param("correlationId")

	resp, err := m.client.GetMovieStatus(c.Request.Context(), &proto.GetMovieStatusRequest{CorrelationId: correlationID})
	if err != nil {
		slog.Error("GetMovieStatus error", slog.Any("error", err))
		c.JSON(grpcErrorToHTTP(err), gin.H{"error": err.Error()})
		return
	}

	switch resp.Status {
	case "completed":
		c.Header("Location", fmt.Sprintf("/api/v1/movies/%d", resp.Movie.Id))
		c.JSON(http.StatusOK, resp.Movie)
	case "failed":
		c.JSON(http.StatusUnprocessableEntity, gin.H{"status": "failed", "error": resp.Error})
	default:
		c.JSON(http.StatusAccepted, gin.H{"status": "pending"})
	}
}

// DeleteMovie godoc
// @Summary      Deletar filme
// @Description  Remove permanentemente um filme do banco de dados
// @Tags         Movies
// @Param        id   path      int     true   "ID do filme a deletar"  minimum(1)
// @Accept       json
// @Produce      json
// @Success      204                                                     "Filme deletado com sucesso"
// @Failure      400  {object}  map[string]string                    "ID inválido"
// @Failure      404  {object}  map[string]string                    "Filme não encontrado"
// @Failure      500  {object}  map[string]string                    "Erro interno do servidor"
// @Router       /movies/{id} [delete]
func (m *MovieHandler) DeleteMovie(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID is required"})
		return
	}

	idInt, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	_, err = m.client.DeleteMovie(c.Request.Context(), &proto.DeleteMovieRequest{Id: int32(idInt)})

	if err != nil {
		c.JSON(grpcErrorToHTTP(err), gin.H{"error": "Internal server error"})
		return
	}

	c.Status(http.StatusNoContent)
}
