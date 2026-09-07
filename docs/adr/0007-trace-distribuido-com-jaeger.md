# ADR 0007: Trace distribuído com Jaeger — HTTP, gRPC e AMQP numa trace só

- **Status:** Implementado
- **Data:** 2026-09-07
- **Escopo:** `api-gateway`, `movies-service`, `shared`, `infra`/`docker-compose.yml`

## Contexto

Até aqui, entender o caminho de uma requisição por `api-gateway` →
`movies-service` → MongoDB (ou, no caminho assíncrono, `api-gateway` →
RabbitMQ → `movies-service` → MongoDB) dependia de cruzar logs de dois
processos manualmente, sem nenhum jeito de saber, olhando só pro
`api-gateway`, quanto tempo o `movies-service` gastou processando uma
chamada específica. Implementei trace distribuído com OpenTelemetry +
Jaeger para resolver exatamente isso: uma trace só, por requisição, que
atravessa os dois serviços e as três formas de transporte que este
projeto usa (HTTP, gRPC, AMQP).

## Decisão

### 1. OpenTelemetry + Jaeger via OTLP, não o exporter Jaeger nativo

Usei `go.opentelemetry.io/otel` (SDK) exportando via
`otlptracegrpc` para o Jaeger — o Jaeger moderno (≥1.35) recebe OTLP
nativamente, então não precisei do exporter específico do Jaeger (que o
próprio projeto OpenTelemetry já descontinuou). Um container
`jaegertracing/all-in-one` novo no `docker-compose.yml`, com
armazenamento em memória (traces não sobrevivem a um restart — aceitável
para um projeto de estudo local; produção usaria um backend de verdade,
ver "Alternativas consideradas").

### 2. HTTP e gRPC: instrumentação pronta; AMQP: teve que ser manual

- **HTTP** (`api-gateway`): `otelgin.Middleware("api-gateway")` — cria um
  span por requisição automaticamente.
- **gRPC** (as duas pontas): `grpc.WithStatsHandler(otelgrpc.NewClientHandler())`
  no client (`api-gateway`) e `grpc.StatsHandler(otelgrpc.NewServerHandler())`
  no server (`movies-service`) — propaga o contexto de trace via metadata
  gRPC automaticamente, sem eu precisar tocar em nenhum handler.
- **AMQP** (RabbitMQ): não existe instrumentação pronta e amplamente
  adotada para `amqp091-go`. Uma mensagem na fila não tem um
  request/response vivo pra pendurar um header como HTTP/gRPC têm — o
  publicador e o consumidor têm que concordar explicitamente onde o
  contexto de trace vai. Criei `shared.AMQPHeaderCarrier` (adapta
  `amqp.Table` para a interface `propagation.TextMapCarrier` do
  OpenTelemetry) e usei manualmente: `api-gateway`'s `RabbitMQPublisher.Publish`
  injeta o contexto nos headers da mensagem antes de publicar;
  `movies-service`'s handler do consumidor extrai de `msg.Headers` antes
  de criar seu próprio span. Coloquei o carrier em `shared` (não
  duplicado em cada serviço) porque isso é genuinamente um contrato entre
  as duas pontas — as duas têm que concordar bit a bit em como o dado é
  serializado, o mesmo motivo pelo qual `MoviePublisherMessage` já morava
  lá.

### 3. MongoDB: spans manuais, não uma biblioteca de instrumentação

Tentei usar `go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo`
primeiro. Não é compatível com este projeto: ela só suporta o driver v1
(`go.mongodb.org/mongo-driver`), e `movies-service` usa o v2
(`go.mongodb.org/mongo-driver/v2`) — confirmei isso na hora, o
`go get` trouxe `go.mongodb.org/mongo-driver v1.17.9` como dependência
indireta, e o tipo de monitor que ela devolve (`*event.CommandMonitor` do
pacote `v1/event`) não é aceito por `options.ClientOptions.SetMonitor` do
driver v2 (que espera o tipo equivalente do pacote `v2/event`.) Removi a
dependência (`go get .../otelmongo@none`) e escrevi um helper genérico
pequeno, `withSpan[T any]` (`movies-service/internal/adapters/mongodb/movie_repo.go`),
que embrulha cada método do repositório (`GetMovieByID`, `ListMovies`,
`CreateMovie`, `NextID`, etc., e os equivalentes em `movie_job_repo.go`)
num span nomeado — sem depender de nenhuma biblioteca de terceiros para
isso.

### 4. Amostragem: `AlwaysSample`, uma escolha específica para este projeto

Configurei `sdktrace.AlwaysSample()` nos dois serviços — toda requisição
vira uma trace completa no Jaeger. Isso é adequado aqui (poucas
requisições, quero ver tudo enquanto desenvolvo/demonstro) e seria uma
escolha ruim numa API com tráfego real (custo de armazenamento e
processamento do coletor cresce linearmente com o volume; produção
normalmente usa um sampler probabilístico ou baseado em taxa) — decisão
registrada aqui para não ser esquecida se este projeto crescer.

### 5. Desligamento gradual (graceful shutdown) nos dois processos

`api-gateway` e `movies-service` trocaram `r.Run()`/`grpcSrv.Serve()`
bloqueante por um `http.Server`/`grpcSrv.GracefulStop()` que espera
`SIGTERM`/`SIGINT` explicitamente antes de encerrar. Motivo prático: o
exportador de trace usa um `BatchSpanProcessor` (span não é exportado na
hora, é acumulado e enviado em lote); sem desligar de propósito e dar
uma chance pro `TracerProvider.Shutdown` rodar, um `docker stop` mataria
o processo com spans ainda no buffer, nunca exportados — o oposto de
"perder o mínimo de trace possível" ao encerrar de forma limpa. Não é uma
mudança sofisticada (não fiz nada com conexões HTTP em voo, por
exemplo), só o suficiente pra essa garantia específica valer.

## O que confirmei de verdade, contra o Jaeger real

Não bastava o código compilar — testei os dois caminhos reais deste
projeto e conferi a trace resultante via API do Jaeger
(`GET /api/traces`), não só a UI:

**Caminho síncrono** (`GET /movies/{id}`):
```
[api-gateway]     GET /api/v1/movies/:id           (18.5ms)
[api-gateway]       movies.MovieService/GetMovie    (13.1ms)  ← client gRPC
[movies-service]     movies.MovieService/GetMovie   (8.3ms)   ← server gRPC
[movies-service]       mongodb.GetMovieByID         (6.9ms)
```

**Caminho assíncrono** (`POST /movies` → fila → consumidor):
```
[api-gateway]     POST /api/v1/movies              (146us)
[api-gateway]       movies_queue publish            (55us)
[movies-service]     movies_queue consume           (21.3ms)  ← via AMQP header carrier
[movies-service]       mongodb.NextID               (5.5ms)
[movies-service]       mongodb.CreateMovie          (12.5ms)
[movies-service]       mongodb.SaveCompleted        (3.1ms)
```

As duas são uma trace só (mesmo `traceID`), com relação pai-filho correta
em cada span — confirmando que a propagação de contexto funciona nos três
transportes, inclusive o que eu tive que implementar na mão (AMQP).

## Consequências

**Positivas**
- Uma requisição, uma trace, atravessando os dois serviços e os três
  transportes — o problema original (cruzar logs de dois processos na
  mão) está resolvido.
- `withSpan` deixa instrumentar um método Mongo novo uma linha de
  trabalho, não um bloco de boilerplate repetido.
- Descobri e documentei, de passagem, que a instrumentação oficial de
  Mongo do ecossistema OpenTelemetry ainda não acompanhou a v2 do driver
  — útil saber antes de tentar de novo no futuro achando que "deve ter
  sido só um erro meu".

**Negativas / pendências que assumi conscientemente**
- Armazenamento em memória do Jaeger — reiniciar o container derruba
  todo o histórico de traces. Aceitável para uso local; um backend real
  (Elasticsearch, Cassandra, ou o Jaeger operado por um provedor) seria
  necessário para reter histórico.
- `AlwaysSample` não é uma configuração de produção — ver "Decisão" nº 4.
- Cada tentativa de retry no `resilience.Client`/`resilience.Publisher`
  (ADR 0006) gera seu próprio span "movies_queue publish"/RPC gRPC como
  span irmão, não agrupado sob um span "tentativa N" explícito — dá pra
  ver múltiplas tentativas na mesma trace, mas não tão organizado quanto
  poderia ser. Não mexi nisso agora; é uma melhoria pontual na
  instrumentação do pacote `resilience`, não um problema de propagação de
  contexto.
- Não escrevi teste automatizado para a propagação de contexto via AMQP
  (precisaria de um coletor OTLP real ou um mock de exporter capturando
  spans) — validei manualmente contra o Jaeger real, com a trace completa
  reproduzida acima, mas isso não pega uma regressão futura sozinho.

## Alternativas consideradas

- **Exporter Jaeger nativo (`otlptracegrpc` é o que uso; existiu também
  um exporter `jaeger.New(...)` específico) em vez de OTLP:** o exporter
  específico do Jaeger foi descontinuado no SDK do OpenTelemetry Go —
  OTLP é o caminho recomendado hoje, e o próprio Jaeger recebe OTLP
  nativamente desde a versão 1.35. Não havia razão para usar o caminho
  descontinuado.
- **`otelmongo` (instrumentação oficial) para os spans do Mongo:**
  descartada por incompatibilidade real com o driver v2, não por
  preferência — ver "Decisão" nº 3.
- **Spans "link" em vez de "filho" para o span de consumo AMQP** (a
  recomendação formal do OpenTelemetry para mensageria assíncrona, já que
  o consumo acontece muito depois da publicação e não deveria contar
  como parte da duração do span pai): optei por uma continuação direta
  (filho) porque é mais simples de implementar e a trace resultante no
  Jaeger já conta a história pretendida (uma trace só, ordem clara) — a
  diferença entre "link" e "filho" importa mais em sistemas com filas de
  latência alta/variável, onde a duração do span pai ficaria artificialmente
  inflada; neste projeto, o tempo entre publicar e consumir é
  tipicamente sub-segundo (ver `docs/performance/README.md`, Fase 2), então
  a distorção é pequena. Registrado como simplificação consciente, não
  como desconhecimento da recomendação.
- **Coletor Jaeger com armazenamento persistente desde já
  (Elasticsearch, por exemplo):** descartado — adicionaria outro
  container e outra peça de infraestrutura para manter, sem necessidade
  real num projeto de estudo local onde a trace de interesse normalmente
  é a que acabou de ser gerada.

## Validação

- `go build ./...`, `go vet ./...` e `go test ./...` limpos nos 4
  módulos do workspace (incluindo `shared`, que ganhou a dependência de
  `amqp091-go` para o carrier).
- Confirmado, via `docker compose up`, que os dois serviços logam
  `tracing ligado` na inicialização e aparecem em
  `GET /api/services` do Jaeger.
- Reproduzidas as duas traces completas mostradas acima, direto da API
  do Jaeger (`GET /api/traces`), não só visualmente na UI — números e
  relações pai-filho conferidos.
- Dataset de seed confirmado intacto (28.451 documentos) depois de
  limpar os filmes de teste usados para gerar as traces.

## Itens descobertos durante a implementação, fora do escopo desta ADR

- Retries do pacote `resilience` (ADR 0006) não agrupam suas tentativas
  sob um span "tentativa" comum — ver "Negativas" acima.
- `movies-service` ainda não expõe métricas Prometheus (só ganhou
  tracing agora) — pendência já registrada desde a ADR 0003.
- Se a instrumentação oficial de Mongo (`otelmongo`) ganhar suporte ao
  driver v2 no futuro, trocar os spans manuais por ela adicionaria
  atributos padronizados (nome do banco, da coleção, do comando) que os
  spans manuais atuais não têm — não fiz isso agora porque a biblioteca
  não suporta, não por preferência.
