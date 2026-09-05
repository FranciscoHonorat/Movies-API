package grpcserver_test

import (
	"context"
	"errors"
	"testing"

	grpcserver "movies-service/internal/adapters/grpc-server"
	"movies-service/internal/core/domain/entity"
	errD "movies-service/internal/core/domain/err-d"
	"movies-service/internal/core/port/output"

	"github.com/FranciscoHonorat/movies/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MockMovieService struct {
	mock.Mock
}

func (m *MockMovieService) GetMovieByID(ctx context.Context, id int) (*entity.MovieEntity, error) {
	args := m.Called(ctx, id)
	if res := args.Get(0); res != nil {
		return res.(*entity.MovieEntity), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockMovieService) ListMovies(ctx context.Context, filters output.Listfilters, pagination output.Pagination, sorting output.Sorting) ([]*entity.MovieEntity, error) {
	args := m.Called(ctx, filters, pagination, sorting)
	if res := args.Get(0); res != nil {
		return res.([]*entity.MovieEntity), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockMovieService) CountMovies(ctx context.Context, filters output.Listfilters) (int, error) {
	args := m.Called(ctx, filters)
	return args.Int(0), args.Error(1)
}

func (m *MockMovieService) CreateMovie(ctx context.Context, movie *entity.MovieEntity) (*entity.MovieEntity, error) {
	args := m.Called(ctx, movie)
	if res := args.Get(0); res != nil {
		return res.(*entity.MovieEntity), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockMovieService) DeleteMovie(ctx context.Context, id int) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockMovieService) NextMovieID(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func createTestMovie(t *testing.T, id int, title, year string) *entity.MovieEntity {
	t.Helper()
	movie, err := entity.NewMovieEntity(id, title, year)
	require.NoError(t, err)
	return movie
}

func assertGRPCError(t *testing.T, err error, expectedCode codes.Code) {
	t.Helper()
	require.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok, "esperado erro gRPC do tipo status.Status")
	assert.Equal(t, expectedCode, st.Code())
}

func TestServer(t *testing.T) {
	t.Run("GetMovieById", func(t *testing.T) {
		validMovie := createTestMovie(t, 1, "Inception", "2010")

		tests := []struct {
			name         string
			req          *proto.GetMovieRequest
			setupMock    func(m *MockMovieService)
			expectedResp *proto.GetMovieResponse
			expectedCode codes.Code
			wantErr      bool
		}{
			{
				name: "Happy Path: Sucesso ao buscar filme por ID",
				req:  &proto.GetMovieRequest{Id: 1},
				setupMock: func(m *MockMovieService) {
					m.On("GetMovieByID", mock.Anything, 1).Return(validMovie, nil)
				},
				expectedResp: &proto.GetMovieResponse{
					Movie: &proto.Movie{
						Id:    1,
						Title: "Inception",
						Year:  "2010",
					},
				},
				wantErr: false,
			},
			{
				name: "Sad Path: Erro retornado pelo serviço",
				req:  &proto.GetMovieRequest{Id: 999},
				setupMock: func(m *MockMovieService) {
					m.On("GetMovieByID", mock.Anything, 999).Return(nil, errD.ErrMovieNotFound)
				},
				wantErr:      true,
				expectedCode: codes.NotFound,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockService := new(MockMovieService)
				tt.setupMock(mockService)

				server := grpcserver.NewServer(mockService)
				resp, err := server.GetMovieById(context.Background(), tt.req)

				if tt.wantErr {
					assertGRPCError(t, err, tt.expectedCode)
					assert.Nil(t, resp)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expectedResp, resp)
				}
				mockService.AssertExpectations(t)
			})
		}
	})

	t.Run("ListMovies", func(t *testing.T) {
		m1 := createTestMovie(t, 1, "Inception", "2010")
		m2 := createTestMovie(t, 2, "Interstellar", "2014")

		tests := []struct {
			name         string
			req          *proto.ListMovieRequest
			setupMock    func(m *MockMovieService)
			expectedResp *proto.ListMovieResponse
			expectedCode codes.Code
			wantErr      bool
		}{
			{
				name: "Happy Path: Listar filmes com filtros e paginação",
				req:  &proto.ListMovieRequest{Title: "Inception", Page: 1, Limit: 10, SortBy: "year"},
				setupMock: func(m *MockMovieService) {
					filters := output.Listfilters{Title: "Inception"}
					pagination := output.Pagination{Page: 1, Limit: 10}
					sorting := output.Sorting{SortBy: "year"}
					m.On("ListMovies", mock.Anything, filters, pagination, sorting).
						Return([]*entity.MovieEntity{m1, m2}, nil)
				},
				expectedResp: &proto.ListMovieResponse{
					Movie: []*proto.Movie{
						{Id: 1, Title: "Inception", Year: "2010"},
						{Id: 2, Title: "Interstellar", Year: "2014"},
					},
				},
				wantErr: false,
			},
			{
				name: "Sad Path: Erro na execução da listagem",
				req:  &proto.ListMovieRequest{Page: 1, Limit: 10},
				setupMock: func(m *MockMovieService) {
					m.On("ListMovies", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
						Return(nil, errors.New("erro de conexao no banco"))
				},
				wantErr:      true,
				expectedCode: codes.Internal,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockService := new(MockMovieService)
				tt.setupMock(mockService)

				server := grpcserver.NewServer(mockService)
				resp, err := server.ListMovie(context.Background(), tt.req)

				if tt.wantErr {
					assertGRPCError(t, err, tt.expectedCode)
					assert.Nil(t, resp)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expectedResp, resp)
				}
				mockService.AssertExpectations(t)
			})
		}
	})

	t.Run("CreateMovie", func(t *testing.T) {
		createdMovie := createTestMovie(t, 1, "Tenet", "2020")

		tests := []struct {
			name         string
			req          *proto.CreateMovieRequest
			setupMock    func(m *MockMovieService)
			expectedResp *proto.CreateMovieResponse
			expectedCode codes.Code
			wantErr      bool
		}{
			{
				name: "Happy Path: Criação bem sucedida",
				req:  &proto.CreateMovieRequest{Title: "Tenet", Year: "2020"},
				setupMock: func(m *MockMovieService) {
					m.On("NextMovieID", mock.Anything).Return(1, nil)
					m.On("CreateMovie", mock.Anything, mock.AnythingOfType("*entity.MovieEntity")).
						Return(createdMovie, nil)
				},
				expectedResp: &proto.CreateMovieResponse{
					Movie: &proto.Movie{
						Id:    1,
						Title: "Tenet",
						Year:  "2020",
					},
				},
				wantErr: false,
			},
			{
				name: "Sad Path: Dados inválidos no serviço",
				req:  &proto.CreateMovieRequest{Title: "Tenet", Year: "2020"},
				setupMock: func(m *MockMovieService) {
					m.On("NextMovieID", mock.Anything).Return(1, nil)
					m.On("CreateMovie", mock.Anything, mock.AnythingOfType("*entity.MovieEntity")).
						Return(nil, errD.ErrInvalidMovieData) // Ou outro erro mapeado para InvalidArgument
				},
				wantErr:      true,
				expectedCode: codes.InvalidArgument,
			},
			{
				name: "Sad Path: Erro no repositório/serviço durante a criação",
				req:  &proto.CreateMovieRequest{Title: "Dunkirk", Year: "2017"},
				setupMock: func(m *MockMovieService) {
					m.On("NextMovieID", mock.Anything).Return(1, nil)
					m.On("CreateMovie", mock.Anything, mock.AnythingOfType("*entity.MovieEntity")).
						Return(nil, errors.New("duplicado"))
				},
				wantErr:      true,
				expectedCode: codes.Internal,
			},
			{
				name: "Sad Path: Erro ao gerar o próximo ID",
				req:  &proto.CreateMovieRequest{Title: "Dunkirk", Year: "2017"},
				setupMock: func(m *MockMovieService) {
					m.On("NextMovieID", mock.Anything).Return(0, errors.New("falha ao gerar id"))
				},
				wantErr:      true,
				expectedCode: codes.Internal,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockService := new(MockMovieService)
				tt.setupMock(mockService)

				server := grpcserver.NewServer(mockService)
				resp, err := server.CreateMovie(context.Background(), tt.req)

				if tt.wantErr {
					assertGRPCError(t, err, tt.expectedCode)
					assert.Nil(t, resp)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expectedResp, resp)
				}
				mockService.AssertExpectations(t)
			})
		}
	})

	t.Run("DeleteMovie", func(t *testing.T) {
		tests := []struct {
			name         string
			req          *proto.DeleteMovieRequest
			setupMock    func(m *MockMovieService)
			expectedResp *proto.DeleteMovieResponse
			expectedCode codes.Code
			wantErr      bool
		}{
			{
				name: "Happy Path: Deleção bem sucedida",
				req:  &proto.DeleteMovieRequest{Id: 10},
				setupMock: func(m *MockMovieService) {
					m.On("DeleteMovie", mock.Anything, 10).Return(nil)
				},
				expectedResp: &proto.DeleteMovieResponse{Success: true},
				wantErr:      false,
			},
			{
				name: "Sad Path: Erro ao deletar filme",
				req:  &proto.DeleteMovieRequest{Id: 99},
				setupMock: func(m *MockMovieService) {
					m.On("DeleteMovie", mock.Anything, 99).Return(errD.ErrMovieNotFound)
				},
				wantErr:      true,
				expectedCode: codes.NotFound,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockService := new(MockMovieService)
				tt.setupMock(mockService)

				server := grpcserver.NewServer(mockService)
				resp, err := server.DeleteMovie(context.Background(), tt.req)

				if tt.wantErr {
					assertGRPCError(t, err, tt.expectedCode)
					assert.Nil(t, resp)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expectedResp, resp)
				}
				mockService.AssertExpectations(t)
			})
		}
	})
}
