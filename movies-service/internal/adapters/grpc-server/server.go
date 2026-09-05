package grpcserver

import (
	"context"
	"movies-service/internal/core/domain/entity"
	"movies-service/internal/core/port/input"
	"movies-service/internal/core/port/output"

	"github.com/FranciscoHonorat/movies/proto"
)

type Server struct {
	proto.UnimplementedMovieServiceServer
	service input.MovieService
}

func NewServer(service input.MovieService) *Server {
	return &Server{
		service: service,
	}
}

func (s *Server) GetMovieById(ctx context.Context, req *proto.GetMovieRequest) (*proto.GetMovieResponse, error) {
	movie, err := s.service.GetMovieByID(ctx, int(req.Id))
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &proto.GetMovieResponse{
		Movie: &proto.Movie{
			Id:    int(movie.GetID()),
			Title: movie.GetTitle(),
			Year:  movie.GetYear(),
		},
	}, nil
}

func (s *Server) ListMovie(ctx context.Context, req *proto.ListMovieRequest) (*proto.ListMovieResponse, error) {
	filters := output.Listfilters{
		Title: req.Title,
		Year:  req.Year,
	}
	pagination := output.Pagination{
		Page:  int(req.Page),
		Limit: int(req.Limit),
	}
	sorting := output.Sorting{
		SortBy: req.SortBy,
	}
	movies, err := s.service.ListMovies(ctx, filters, pagination, sorting)
	if err != nil {
		return nil, toGRPCError(err)
	}

	var protoMovies []*proto.Movie
	for _, m := range movies {
		protoMovies = append(protoMovies, &proto.Movie{
			Id:    int(m.GetID()),
			Title: m.GetTitle(),
			Year:  m.GetYear(),
		})
	}
	return &proto.ListMovieResponse{Movie: protoMovies}, nil
}

func (s *Server) CreateMovie(ctx context.Context, req *proto.CreateMovieRequest) (*proto.CreateMovieResponse, error) {
	id, err := s.service.NextMovieID(ctx)
	if err != nil {
		return nil, toGRPCError(err)
	}

	movie, err := entity.NewMovieEntity(id, req.Title, req.Year)
	if err != nil {
		return nil, toGRPCError(err)
	}

	createdMovie, err := s.service.CreateMovie(ctx, movie)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &proto.CreateMovieResponse{
		Movie: &proto.Movie{
			Id:    int(createdMovie.GetID()),
			Title: createdMovie.GetTitle(),
			Year:  createdMovie.GetYear(),
		},
	}, nil
}

func (s *Server) DeleteMovie(ctx context.Context, req *proto.DeleteMovieRequest) (*proto.DeleteMovieResponse, error) {
	err := s.service.DeleteMovie(ctx, int(req.Id))
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &proto.DeleteMovieResponse{Success: true}, nil
}
