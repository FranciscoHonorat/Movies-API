# ADR 0001: Correção da criação assíncrona de filmes, geração de ID e seed

- **Status:** Implementado
- **Data:** 2026-09-05
- **Escopo:** Fase 0 do roadmap de auditoria (re-api-books)

## Contexto

A auditoria do repositório identificou que o fluxo de criação de filme estava
funcionalmente quebrado: `grpc-server/server.go` construía toda entidade com
`ID=1` fixo, e o repositório persiste via `upsert` por `_id` — ou seja, toda
"criação" sobrescrevia o mesmo documento. Um segundo bug, não listado na
auditoria original, foi descoberto durante a implementação: o consumidor
RabbitMQ (`movies-service/cmd/main.go`) desserializava a mensagem da fila
diretamente em `entity.MovieEntity`, cujos campos são *value objects*
(`valueobjects.MovieID`, `MovieTitle`, `MovieYear`), não tipos primitivos —
`json.Unmarshal` falhava para toda mensagem, então o caminho de criação via
fila nunca chegava a chamar `CreateMovie`. Combinados, os dois bugs faziam da
criação de filmes uma operação que não fazia nada de útil.

Também fazia parte da Fase 0: o seed nunca rodava (mismatch de tipo entre
`movies.json` e `MongoSeed.Year`), os testes de integração do Mongo travavam
por minutos sem uma instância local, `.env.example` divergia do
`docker-compose.yml`, as portas dos serviços ignoravam as variáveis de
ambiente, e o campo `Total` da listagem nunca era populado.

## Decisão

1. **Geração de ID por contador atômico no Mongo, não por ID vindo do
   cliente.** `movies-service/internal/adapters/mongodb/movie_repo.go` ganhou
   um método `NextID(ctx)` que faz `$inc` atômico em um documento de uma
   coleção `counters` dedicada. Na primeira chamada (guardada por
   `sync.Once`), o contador é inicializado a partir do maior `_id` já
   existente na coleção `movies` (via `$max` em upsert), para nunca colidir
   com os ~28 mil IDs não contíguos do dataset de seed. Optamos por manter o
   ID como inteiro sequencial em vez de migrar para `ObjectID`/UUID — trocar
   o tipo do ID quebraria o contrato `int32` do proto, o formato da API HTTP
   e o value object `MovieID` existente, mudança grande demais para uma
   correção de Fase 0.
2. A geração de ID passou a ser responsabilidade do `movies-service`
   (`service.NextMovieID`, chamado tanto pelo handler gRPC `CreateMovie`
   quanto pelo consumidor RabbitMQ) — nenhum cliente (gateway ou mensagem de
   fila) escolhe o ID de um filme novo.
3. O consumidor RabbitMQ agora desserializa para `shared.MoviePublisherMessage`
   (o DTO real publicado pelo gateway) e só então constrói a entidade de
   domínio com o ID gerado. `movies-service/go.mod` ganhou a dependência do
   módulo `shared`.
4. `MongoSeed.Year` corrigido de `int` para `string`, batendo com
   `movies.json` e com o schema que a aplicação já escrevia
   (`movieRepository.Year string`) — eliminando também a inconsistência de
   schema entre documentos seedados e documentos criados pela aplicação.
   Criado o fixture `movies-service/internal/adapters/seed/testdata/movies.json`
   que faltava (o teste de seed nunca tinha esse arquivo).
5. Testes de integração (`movie_repo_test.go`, `mongo_seed_test.go`) passaram
   a usar `SetServerSelectionTimeout(2 * time.Second)` — sem MongoDB local a
   suíte falha em segundos, não trava por minutos. Isso resolve o
   travamento; fazer a suíte *passar* de forma confiável em CI é
   responsabilidade da Fase 1 (subir Mongo/RabbitMQ como serviços no
   pipeline).
6. Validando a correção acima com um MongoDB real, apareceu um terceiro bug,
   também não catalogado na auditoria original: o subteste "happy path" de
   `TestSeed` nunca limpava a coleção `testdb.movies` depois de semeá-la,
   então o subteste seguinte ("sad path") contava documentos que sobraram do
   anterior. Corrigido com o mesmo `collection.Drop` já usado no início do
   próprio subteste "happy path".
7. `.env.example` reescrito para bater exatamente com as variáveis esperadas
   por `docker-compose.yml`.
8. `movies-service/cmd/main.go` e `api-gateway/cmd/main.go` passaram a ler a
   porta de `GRPC_PORT`/`HTTP_PORT`, com fallback para o valor hardcoded
   anterior — as variáveis de ambiente do compose/K8s agora têm efeito real.
9. O campo `Total` de `ListMovieStruct` foi **removido**, em vez de
   populado artificialmente. `CountMovies` existe em service/repositório mas
   não está exposto no contrato gRPC (`ListMovieResponse` não tem campo
   `total`); expor esse dado corretamente exige alterar o `.proto` e
   regenerar o código com `protoc`, que não está disponível neste ambiente.
   Fica registrado como item pendente (ver "Itens descobertos" abaixo).

## Consequências

**Positivas**
- Cada criação de filme agora produz um documento novo, com ID único, sem
  sobrescrever dados existentes — confirmado criando 3 filmes contra um
  MongoDB real (IDs `501`, `502`, `503`, continuando após um documento
  seedado com `_id=500`, sem alterá-lo).
- O caminho assíncrono (gateway → RabbitMQ → movies-service) agora consegue
  de fato desserializar e criar o filme — antes falhava silenciosamente em
  todo request.
- `go test ./...` do `movies-service` passa integralmente contra um Mongo
  real (`docker run mongo:7`), incluindo os testes de integração antes
  quebrados.
- Portas e variáveis de ambiente documentadas passaram a ter efeito real.

**Negativas / pendências assumidas conscientemente**
- A inicialização do contador (`sync.Once` + varredura do maior `_id`) roda
  na primeira chamada de `NextID`, usando o contexto dessa primeira
  requisição — aceitável para uma correção pontual, mas o ideal a médio
  prazo é mover essa inicialização para o boot do serviço, de forma
  explícita e testável isoladamente.
- `Total` saiu da resposta em vez de ser implementado corretamente; requer
  alterar `proto/movies.proto` e rodar `protoc` (não instalado aqui).
- Testes de integração falham rápido sem Mongo, mas continuam falhando (não
  fazem skip) — decisão deliberada para não mascarar infraestrutura
  quebrada; CI real com Mongo/RabbitMQ é a Fase 1.

## Alternativas consideradas

- **UUID/ObjectID em vez de contador sequencial:** mais robusto para múltiplas
  réplicas do `movies-service` (sem ponto único de contenção no documento
  contador), mas exige mudar o tipo do ID em todo o contrato (proto, HTTP,
  value object) e no dataset de seed. Descartado para a Fase 0; vale
  reconsiderar se o serviço passar a rodar com múltiplas réplicas gravando
  concorrentemente.
- **`t.Skip` em vez de falha rápida quando o Mongo está indisponível:**
  descartado por ora para manter os testes honestos sobre infraestrutura
  quebrada; pode valer a pena para conveniência de desenvolvimento local uma
  vez que a Fase 1 garanta Mongo real em CI.

## Validação

- `go build ./...` e `go vet ./...` limpos nos 4 módulos do workspace
  (`movies-service`, `api-gateway`, `proto`, `shared`).
- Testes unitários (mockados) passam sem infraestrutura.
- Suíte completa (`go test -p 1 ./...`) passa contra MongoDB 7 real, subido
  via Docker só para esta validação e removido em seguida.
- Script manual confirmou geração de IDs distintos e não-sobrescrita de
  documento seedado (ver "Consequências").

## Itens descobertos durante a implementação, fora do escopo desta ADR

- **Isolamento de banco entre pacotes de teste:** `mongodb` e `seed` usam o
  mesmo nome de banco (`testdb`) no Mongo local. Rodando `go test ./...`
  sem `-p 1` (paralelismo padrão entre pacotes), os dois pacotes disputam a
  mesma coleção `testdb.movies` e os testes ficam instáveis. Recomendação
  para a Fase 1: banco efêmero por pacote/execução, ou usar `-p 1` até lá.
- Regenerar o proto com `total` em `ListMovieResponse` quando houver
  `protoc` disponível no ambiente de desenvolvimento/CI.
