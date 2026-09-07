# ADR 0002: Correção do endpoint de criação de filmes, mapeamento de erros de "não encontrado" e CI com MongoDB real

- **Status:** Implementado
- **Data:** 2026-09-07
- **Escopo:** Fase 1 do roadmap de auditoria (re-api-books), continuação da ADR 0001

## Contexto

Uma nova auditoria do repositório, feita após a ADR 0001 (Fase 0) e após o
fluxo de status assíncrono (`GetMovieStatus`, `MovieJobRepository`) já estar
implementado ponta a ponta, encontrou um bug crítico não coberto por nenhum
teste existente: a tag de binding do Gin em `CreateMovieStruct.Title`
(`api-gateway/internal/handlers/movie-handler.go`) estava escrita como
`binding:"required, max=300"` — com um espaço depois da vírgula. O pacote
`go-playground/validator` (usado internamente pelo Gin) não tolera espaço
entre as tags: ele interpreta o segundo token como uma tag chamada `" max"`
(com espaço), que não existe, e entra em **pânico** — `panic: Undefined
validation function ' max' on field 'Title'` — a cada chamada de
`ShouldBindJSON`. Como o Gin roda com o middleware `Recovery()` por padrão
(`gin.Default()`), o processo não derruba, mas **toda requisição
`POST /api/v1/movies` retorna 500**, mesmo com payload válido. Confirmado
reproduzindo o binding isoladamente e com um teste de handler real (ver
"Validação"). Esse era o único caminho de criação de filme via API — o
endpoint estava, na prática, inutilizável.

O `api-gateway` não tinha nenhum arquivo de teste antes desta ADR, o que
explica por que o bug não foi pego antes de chegar a este ponto do roadmap.

Um segundo bug, de menor severidade mas igualmente real, foi encontrado no
`movies-service`: `movie_repo.go` (`GetMovieByID` e `DeleteMovie`) devolvia
o erro cru do driver do Mongo (`mongo.ErrNoDocuments`) em vez de
`errD.ErrMovieNotFound`. `toGRPCError` (`grpc-server/errors.go`) só reconhece
erros de domínio (`errD.*`) via `errors.Is`; um `mongo.ErrNoDocuments` cai no
`default` e vira `codes.Internal` → HTTP 500, não o 404 documentado no
Swagger. Os testes de integração existentes (`movie_repo_test.go`) só
verificavam `assert.Error(t, err)`, sem checar a identidade do erro, então a
regressão nunca foi detectada.

Por fim, a pendência já registrada na ADR 0001 — "CI real com Mongo/RabbitMQ
é a Fase 1" — seguia aberta: `.github/workflows/go.yml` roda
`go test -v ./...` para `movies-service` sem nenhum MongoDB disponível, então
os testes de integração do pacote `mongodb` falham de forma determinística
em todo push/PR (reproduzido localmente: 6 subtestes falhando com "connection
refused"). E o adapter mais novo do repositório, `movie_job_repo.go` (o
repositório Mongo por trás do status assíncrono), foi implementado e mergeado
sem nenhum teste, ao contrário do seu irmão `movie_repo.go`.

## Decisão

1. **Corrigida a tag de binding** para `binding:"required,max=300"` (sem
   espaço). A validação de tamanho no Gin é mantida (não só a validação de
   domínio em `valueobjects.MovieTitle`) porque falhar rápido no gateway
   evita publicar uma mensagem inválida na fila que só vai falhar
   assincronamente do outro lado — o cliente recebe 400 na hora, em vez de
   200/202 seguido de um `status: failed` ao consultar depois.
2. **`GetMovieByID` e `DeleteMovie`, em `movies-service/internal/adapters/mongodb/movie_repo.go`,
   agora traduzem `mongo.ErrNoDocuments` para `errD.ErrMovieNotFound`** antes
   de devolver o erro. A tradução acontece no adapter, na fronteira com a
   infraestrutura — é responsabilidade dele não vazar tipos de erro
   específicos do Mongo para as camadas de cima (serviço, gRPC, HTTP), que já
   sabem lidar com `errD.ErrMovieNotFound` (`toGRPCError` já tinha esse
   `case`; só faltava alguém emitir o erro certo).
3. **`.github/workflows/go.yml` ganhou um `services: mongo:` (imagem
   `mongo:7`, porta `27017`, com healthcheck)** no job `build`, e o passo de
   teste passou a rodar `go test -p 1 -v ./...` (o `-p 1` evita a
   instabilidade, já documentada na ADR 0001, de `mongodb` e `seed`
   disputando a mesma coleção `testdb.movies` em paralelo). O serviço é
   definido no job da matrix inteira (`api-gateway` e `movies-service`) em
   vez de só no leg de `movies-service` — `services:` do GitHub Actions é
   escopado ao job, não dá para condicionar por valor de matrix sem duplicar
   o job inteiro; o custo de subir um Mongo não utilizado no leg do
   `api-gateway` é desprezível frente à complexidade de separar os jobs.
4. **`movie_job_repo_test.go` criado**, seguindo o mesmo padrão de
   `movie_repo_test.go` (testes de integração contra Mongo real em
   `testdb.movie_jobs`): cobre `SaveCompleted`, `SaveFailed`, o caso
   "pending" para um `correlation_id` nunca visto, e o upsert de um job que
   falhou e depois completou (retry).
5. **`api-gateway/internal/handlers/movie_handler_test.go` criado** — o
   primeiro arquivo de teste do módulo `api-gateway`. Cobre `CreateMovie`
   com um `MockMoviePublisher` (mesmo padrão `stretchr/testify/mock` já usado
   em `movies-service`): payload válido (teste de regressão explícito para o
   bug do item 1), título ausente, título acima de 300 caracteres, e erro do
   publisher propagado como 500. `stretchr/testify` foi adicionado como
   dependência direta do módulo `api-gateway` (`go get` + `go mod tidy`).
6. Os testes "Sad Path" de `GetMovieByID`/`DeleteMovie` em
   `movie_repo_test.go` foram reforçados para checar `errors.Is(err,
   errD.ErrMovieNotFound)` em vez de só `assert.Error`, para que uma futura
   regressão como a do item 2 seja pega automaticamente.

## Consequências

**Positivas**
- `POST /api/v1/movies` volta a funcionar — confirmado com teste de handler
  real (não só leitura de código) que exercita `ShouldBindJSON` de ponta a
  ponta.
- `GetMovie`/`DeleteMovie` para um ID inexistente agora devolvem 404, como
  documentado no Swagger, em vez de 500.
- CI deixa de falhar deterministicamente em todo push: os testes de
  integração do `movies-service` rodam contra um Mongo real no runner.
- `api-gateway` ganha sua primeira suíte de testes; `movie_job_repo.go` deixa
  de ser o único adapter Mongo sem cobertura.

**Negativas / pendências assumidas conscientemente**
- O serviço `mongo` no `go.yml` sobe também para o leg `api-gateway` da
  matrix, sem uso — aceito pela simplicidade de manter um único job (ver
  item 3 da Decisão).
- RabbitMQ continua sem teste de integração contra um broker real (nem no
  `movies-service`, nem no `api-gateway`) — o publisher e o consumer só têm
  cobertura indireta via os testes de serviço/handler com mocks. Fica para
  uma fase futura subir também um `services: rabbitmq:` e testar
  `RabbitMQPublisher`/`Consumer` de fato.
- O healthcheck do `api-gateway` (`GET /health`) continua estático — não
  verifica conectividade com o `movies-service` (gRPC) nem com o RabbitMQ.
  Não é um bug (o endpoint nunca prometeu isso), mas é uma limitação
  descoberta durante esta auditoria, registrada abaixo.

## Alternativas consideradas

- **Remover a validação de tamanho no Gin e confiar só no domínio
  (`valueobjects.MovieTitle`):** mais simples, um lugar só para a regra, mas
  perde o fail-fast síncrono — um título grande demais só falharia depois de
  publicado na fila, e o cliente teria que fazer polling em
  `GET /movies/status/{id}` para descobrir. Descartado: o custo de manter a
  tag correta é menor que o custo de UX de descobrir erros de validação
  simples de forma assíncrona.
- **Separar `movies-service` e `api-gateway` em jobs distintos no
  `go.yml`, com `services: mongo:` só no primeiro:** mais "correto"
  estruturalmente, mas exige duplicar os passos de checkout/setup-go/build
  em dois jobs. Descartado por ora — o workflow é pequeno o bastante para não
  justificar a duplicação; reconsiderar se o `api-gateway` ganhar
  dependências próprias de infraestrutura no futuro.
- **Fazer `t.Skip` nos testes de integração quando o Mongo não está
  acessível** (em vez de continuar falhando rápido): já descartada na ADR
  0001 pelo mesmo motivo — não mascarar infraestrutura quebrada. Com o Mongo
  real agora provisionado em CI, essa alternativa fica ainda menos
  necessária.

## Validação

- `go build ./...` e `go vet ./...` limpos nos 4 módulos do workspace.
- `go test -p 1 ./...` do `movies-service` passa integralmente contra
  MongoDB 7 real (subido via Docker só para esta validação e removido em
  seguida), incluindo os novos testes de `movie_job_repo_test.go` e as
  asserções reforçadas de "not found".
- `go test ./...` do `api-gateway` passa, incluindo o teste de regressão
  `TestCreateMovie_ValidPayloadDoesNotPanic`.
- Reproduzido o bug original antes da correção: um teste isolado com
  `binding:"required, max=300"` e uma chamada real a `ShouldBindJSON`
  disparava `panic: Undefined validation function ' max' on field 'Title'`;
  o mesmo teste, após a correção da tag, passa.
- `.github/workflows/go.yml` validado com `yaml.safe_load` (sintaxe); não
  executado em um runner real do GitHub Actions como parte desta ADR.

## Itens descobertos durante a implementação, fora do escopo desta ADR

- **Healthcheck raso no `api-gateway`:** `GET /health` sempre devolve
  `{"status": "ok"}`, sem checar a conexão gRPC com o `movies-service` nem o
  RabbitMQ. Útil para orquestradores decidirem se o processo está de pé, mas
  não serve como readiness probe real.
- **Credenciais divergentes entre `.env.example` (`MONGO_ROOT_PASSWORD=password`)
  e o `.env` gerado inline em `docker-image.yml` (`MONGO_ROOT_PASSWORD=secret`).**
  Inofensivo hoje porque `docker-image.yml` só builda as imagens (não sobe o
  Mongo), mas vale alinhar para não confundir quem for depurar o build.
- **Sem teste de integração contra RabbitMQ real** — ver "Consequências".
- Os manifests em `k8s/*.yaml` continuam com credenciais em texto plano nas
  specs dos Deployments (`root:password`, `guest:guest`) em vez de
  `Secret`s — aceitável para o estágio atual do projeto (sem cluster real em
  uso), mas não deveria ir para um ambiente de fato compartilhado sem
  `kubectl create secret` + `secretKeyRef`.
