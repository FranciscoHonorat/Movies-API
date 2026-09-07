package service_test

import (
	"context"
	"errors"
	"testing"

	"movies-service/internal/core/domain/entity"
	"movies-service/internal/core/port/output"
	"movies-service/internal/core/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockMovieRepository struct {
	mock.Mock
}

func (m *MockMovieRepository) GetMovieByID(ctx context.Context, id int32) (*entity.MovieEntity, error) {
	args := m.Called(ctx, id)
	if res := args.Get(0); res != nil {
		return res.(*entity.MovieEntity), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockMovieRepository) ListMovies(ctx context.Context, filters output.Listfilters, pagination output.Pagination, sorting output.Sorting) ([]*entity.MovieEntity, error) {
	args := m.Called(ctx, filters, pagination, sorting)
	if res := args.Get(0); res != nil {
		return res.([]*entity.MovieEntity), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockMovieRepository) CountMovies(ctx context.Context, filters output.Listfilters) (int32, error) {
	args := m.Called(ctx, filters)
	return int32(args.Int(0)), args.Error(1)
}

func (m *MockMovieRepository) CreateMovie(ctx context.Context, movie *entity.MovieEntity) (*entity.MovieEntity, error) {
	args := m.Called(ctx, movie)
	if res := args.Get(0); res != nil {
		return res.(*entity.MovieEntity), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockMovieRepository) DeleteMovie(ctx context.Context, id int32) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockMovieRepository) NextID(ctx context.Context) (int32, error) {
	args := m.Called(ctx)
	return int32(args.Int(0)), args.Error(1)
}

type MockMovieJobRepository struct {
	mock.Mock
}

func (m *MockMovieJobRepository) SaveCompleted(ctx context.Context, correlationID string, movieID int32) error {
	args := m.Called(ctx, correlationID, movieID)
	return args.Error(0)
}

func (m *MockMovieJobRepository) SaveFailed(ctx context.Context, correlationID string, errMsg string) error {
	args := m.Called(ctx, correlationID, errMsg)
	return args.Error(0)
}

func (m *MockMovieJobRepository) GetStatus(ctx context.Context, correlationID string) (*output.JobStatus, error) {
	args := m.Called(ctx, correlationID)
	if res := args.Get(0); res != nil {
		return res.(*output.JobStatus), args.Error(1)
	}
	return nil, args.Error(1)
}

func helperNewMovie(t *testing.T, id int32, title, year string) *entity.MovieEntity {
	t.Helper()
	movie, err := entity.NewMovieEntity(id, title, year)
	require.NoError(t, err)
	return movie
}

func TestMovieService(t *testing.T) {
	t.Run("GetMovieByID", func(t *testing.T) {
		validMovie := helperNewMovie(t, 1, "Inception", "2010")

		tests := []struct {
			name      string
			id        int32
			setupMock func(m *MockMovieRepository)
			want      *entity.MovieEntity
			wantErr   bool
		}{
			{
				name: "Happy Path: Sucesso ao buscar por ID",
				id:   1,
				setupMock: func(m *MockMovieRepository) {
					m.On("GetMovieByID", mock.Anything, int32(1)).Return(validMovie, nil)
				},
				want:    validMovie,
				wantErr: false,
			},
			{
				name:      "Sad Path: ID inválido (<= 0)",
				id:        0,
				setupMock: func(m *MockMovieRepository) {},
				want:      nil,
				wantErr:   true,
			},
			{
				name: "Sad Path: Erro retornado pelo repositório",
				id:   99,
				setupMock: func(m *MockMovieRepository) {
					m.On("GetMovieByID", mock.Anything, int32(99)).Return(nil, errors.New("filme não encontrado"))
				},
				want:    nil,
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockRepo := new(MockMovieRepository)
				mockJobs := new(MockMovieJobRepository)
				tt.setupMock(mockRepo)

				svc := service.NewMovieService(mockRepo, mockJobs)
				got, err := svc.GetMovieByID(context.Background(), tt.id)

				if tt.wantErr {
					assert.Error(t, err)
					assert.Nil(t, got)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.want, got)
				}
				mockRepo.AssertExpectations(t)
			})
		}
	})

	t.Run("ListMovies", func(t *testing.T) {
		m1 := helperNewMovie(t, 1, "Inception", "2010")
		m2 := helperNewMovie(t, 2, "Interstellar", "2014")

		filters := output.Listfilters{Title: "Inception"}
		pagination := output.Pagination{Page: 1, Limit: 10}
		sorting := output.Sorting{SortBy: "year"}

		tests := []struct {
			name       string
			filters    output.Listfilters
			pagination output.Pagination
			sorting    output.Sorting
			setupMock  func(m *MockMovieRepository)
			want       []*entity.MovieEntity
			wantErr    bool
		}{
			{
				name:       "Happy Path: Listar filmes com sucesso",
				filters:    filters,
				pagination: pagination,
				sorting:    sorting,
				setupMock: func(m *MockMovieRepository) {
					m.On("ListMovies", mock.Anything, filters, pagination, sorting).
						Return([]*entity.MovieEntity{m1, m2}, nil)
				},
				want:    []*entity.MovieEntity{m1, m2},
				wantErr: false,
			},
			{
				name:       "Sad Path: Erro no repositório ao listar",
				filters:    filters,
				pagination: pagination,
				sorting:    sorting,
				setupMock: func(m *MockMovieRepository) {
					m.On("ListMovies", mock.Anything, filters, pagination, sorting).
						Return(nil, errors.New("erro de conexão com o banco"))
				},
				want:    nil,
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockRepo := new(MockMovieRepository)
				mockJobs := new(MockMovieJobRepository)
				tt.setupMock(mockRepo)

				svc := service.NewMovieService(mockRepo, mockJobs)
				got, err := svc.ListMovies(context.Background(), tt.filters, tt.pagination, tt.sorting)

				if tt.wantErr {
					assert.Error(t, err)
					assert.Nil(t, got)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.want, got)
				}
				mockRepo.AssertExpectations(t)
			})
		}
	})

	t.Run("CountMovies", func(t *testing.T) {
		filters := output.Listfilters{Title: "Tenet"}

		tests := []struct {
			name      string
			filters   output.Listfilters
			setupMock func(m *MockMovieRepository)
			want      int32
			wantErr   bool
		}{
			{
				name:    "Happy Path: Contagem realizada com sucesso",
				filters: filters,
				setupMock: func(m *MockMovieRepository) {
					m.On("CountMovies", mock.Anything, filters).Return(5, nil)
				},
				want:    5,
				wantErr: false,
			},
			{
				name:    "Sad Path: Erro no repositório",
				filters: filters,
				setupMock: func(m *MockMovieRepository) {
					m.On("CountMovies", mock.Anything, filters).Return(0, errors.New("erro ao contar"))
				},
				want:    0,
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockRepo := new(MockMovieRepository)
				mockJobs := new(MockMovieJobRepository)
				tt.setupMock(mockRepo)

				svc := service.NewMovieService(mockRepo, mockJobs)
				got, err := svc.CountMovies(context.Background(), tt.filters)

				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.want, got)
				}
				mockRepo.AssertExpectations(t)
			})
		}
	})

	t.Run("CreateMovie", func(t *testing.T) {
		validMovie := helperNewMovie(t, 1, "Tenet", "2020")
		createdMovie := helperNewMovie(t, 1, "Tenet", "2020")

		tests := []struct {
			name      string
			movie     *entity.MovieEntity
			setupMock func(m *MockMovieRepository)
			want      *entity.MovieEntity
			wantErr   bool
		}{
			{
				name:  "Happy Path: Filme criado com sucesso",
				movie: validMovie,
				setupMock: func(m *MockMovieRepository) {
					m.On("CreateMovie", mock.Anything, mock.Anything).Return(createdMovie, nil)
				},
				want:    createdMovie,
				wantErr: false,
			},
			{
				name:      "Sad Path: Ponteiro de filme nil",
				movie:     nil,
				setupMock: func(m *MockMovieRepository) {},
				want:      nil,
				wantErr:   true,
			},
			{
				name:  "Sad Path: Falha de persistência no repositório",
				movie: validMovie,
				setupMock: func(m *MockMovieRepository) {
					m.On("CreateMovie", mock.Anything, mock.Anything).Return(nil, errors.New("falha ao inserir"))
				},
				want:    nil,
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockRepo := new(MockMovieRepository)
				mockJobs := new(MockMovieJobRepository)
				tt.setupMock(mockRepo)

				svc := service.NewMovieService(mockRepo, mockJobs)
				got, err := svc.CreateMovie(context.Background(), tt.movie)

				if tt.wantErr {
					assert.Error(t, err)
					assert.Nil(t, got)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.want, got)
				}
				mockRepo.AssertExpectations(t)
			})
		}
	})

	t.Run("NextMovieID", func(t *testing.T) {
		t.Run("Happy Path: retorna o próximo ID gerado pelo repositório", func(t *testing.T) {
			mockRepo := new(MockMovieRepository)
			mockJobs := new(MockMovieJobRepository)
			mockRepo.On("NextID", mock.Anything).Return(42, nil)

			svc := service.NewMovieService(mockRepo, mockJobs)
			got, err := svc.NextMovieID(context.Background())

			assert.NoError(t, err)
			assert.Equal(t, int32(42), got)
			mockRepo.AssertExpectations(t)
		})

		t.Run("Sad Path: erro do repositório é propagado", func(t *testing.T) {
			mockRepo := new(MockMovieRepository)
			mockJobs := new(MockMovieJobRepository)
			mockRepo.On("NextID", mock.Anything).Return(0, errors.New("falha ao gerar id"))

			svc := service.NewMovieService(mockRepo, mockJobs)
			got, err := svc.NextMovieID(context.Background())

			assert.Error(t, err)
			assert.Equal(t, int32(0), got)
			mockRepo.AssertExpectations(t)
		})
	})

	t.Run("DeleteMovie", func(t *testing.T) {
		tests := []struct {
			name      string
			id        int32
			setupMock func(m *MockMovieRepository)
			wantErr   bool
		}{
			{
				name: "Happy Path: Deleção bem-sucedida",
				id:   1,
				setupMock: func(m *MockMovieRepository) {
					m.On("DeleteMovie", mock.Anything, int32(1)).Return(nil)
				},
				wantErr: false,
			},
			{
				name:      "Sad Path: ID inválido (<= 0)",
				id:        -5,
				setupMock: func(m *MockMovieRepository) {},
				wantErr:   true,
			},
			{
				name: "Sad Path: Erro no repositório",
				id:   10,
				setupMock: func(m *MockMovieRepository) {
					m.On("DeleteMovie", mock.Anything, int32(10)).Return(errors.New("falha ao deletar"))
				},
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockRepo := new(MockMovieRepository)
				mockJobs := new(MockMovieJobRepository)
				tt.setupMock(mockRepo)

				svc := service.NewMovieService(mockRepo, mockJobs)
				err := svc.DeleteMovie(context.Background(), tt.id)

				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
				mockRepo.AssertExpectations(t)
			})
		}
	})

	t.Run("RecordJobCompleted", func(t *testing.T) {
		mockRepo := new(MockMovieRepository)
		mockJobs := new(MockMovieJobRepository)
		mockJobs.On("SaveCompleted", mock.Anything, "corr-1", int32(1)).Return(nil)

		svc := service.NewMovieService(mockRepo, mockJobs)
		err := svc.RecordJobCompleted(context.Background(), "corr-1", 1)

		assert.NoError(t, err)
		mockJobs.AssertExpectations(t)
	})

	t.Run("RecordJobFailed", func(t *testing.T) {
		mockRepo := new(MockMovieRepository)
		mockJobs := new(MockMovieJobRepository)
		mockJobs.On("SaveFailed", mock.Anything, "corr-1", "title not valid").Return(nil)

		svc := service.NewMovieService(mockRepo, mockJobs)
		err := svc.RecordJobFailed(context.Background(), "corr-1", "title not valid")

		assert.NoError(t, err)
		mockJobs.AssertExpectations(t)
	})

	t.Run("GetJobStatus", func(t *testing.T) {
		completedMovie := helperNewMovie(t, 1, "Tenet", "2020")

		tests := []struct {
			name          string
			correlationID string
			setupMocks    func(jobs *MockMovieJobRepository, repo *MockMovieRepository)
			want          *output.MovieJobStatus
			wantErr       bool
		}{
			{
				name:          "Happy Path: job pendente (sem registro ainda)",
				correlationID: "corr-pending",
				setupMocks: func(jobs *MockMovieJobRepository, repo *MockMovieRepository) {
					jobs.On("GetStatus", mock.Anything, "corr-pending").
						Return(&output.JobStatus{Status: "pending"}, nil)
				},
				want: &output.MovieJobStatus{Status: "pending"},
			},
			{
				name:          "Happy Path: job concluído busca o filme criado",
				correlationID: "corr-done",
				setupMocks: func(jobs *MockMovieJobRepository, repo *MockMovieRepository) {
					jobs.On("GetStatus", mock.Anything, "corr-done").
						Return(&output.JobStatus{Status: "completed", MovieID: 1}, nil)
					repo.On("GetMovieByID", mock.Anything, int32(1)).Return(completedMovie, nil)
				},
				want: &output.MovieJobStatus{Status: "completed", Movie: completedMovie},
			},
			{
				name:          "Happy Path: job falhou propaga a mensagem de erro",
				correlationID: "corr-failed",
				setupMocks: func(jobs *MockMovieJobRepository, repo *MockMovieRepository) {
					jobs.On("GetStatus", mock.Anything, "corr-failed").
						Return(&output.JobStatus{Status: "failed", Error: "title not valid"}, nil)
				},
				want: &output.MovieJobStatus{Status: "failed", Error: "title not valid"},
			},
			{
				name:          "Sad Path: erro no repositório de jobs",
				correlationID: "corr-err",
				setupMocks: func(jobs *MockMovieJobRepository, repo *MockMovieRepository) {
					jobs.On("GetStatus", mock.Anything, "corr-err").
						Return(nil, errors.New("erro de conexão com o banco"))
				},
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockRepo := new(MockMovieRepository)
				mockJobs := new(MockMovieJobRepository)
				tt.setupMocks(mockJobs, mockRepo)

				svc := service.NewMovieService(mockRepo, mockJobs)
				got, err := svc.GetJobStatus(context.Background(), tt.correlationID)

				if tt.wantErr {
					assert.Error(t, err)
					assert.Nil(t, got)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.want, got)
				}
				mockJobs.AssertExpectations(t)
				mockRepo.AssertExpectations(t)
			})
		}
	})
}
