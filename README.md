# Movies-API

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)
![Status](https://img.shields.io/badge/status-projeto%20de%20estudo-lightgrey)

API de filmes em Go, com criação assíncrona via fila de mensagens. Serve
como projeto de estudo de arquitetura de microsserviços, gRPC, mensageria
e — mais recentemente — de teste de carga e observabilidade de verdade,
não só implementação. Todo o histórico de decisões (o quê, por quê, o que
foi descartado e por quê) fica em `docs/`, não só no código.

## Arquitetura

```
                 HTTP/REST                    gRPC
   cliente  ────────────────►  api-gateway  ────────────►  movies-service
                                    │                            │  │
                                    │ publica                    │  └── lê/escreve
                                    ▼                             │      (MongoDB)
                              RabbitMQ  ◄────────────────────────┘
                            (movies_queue)   consome
```

- **`api-gateway`** — API REST (Gin). Recebe requisições HTTP, repassa
  leituras/deleções ao `movies-service` via gRPC, e publica criações na
  fila em vez de criar de forma síncrona.
- **`movies-service`** — servidor gRPC. Dono do MongoDB: filmes, contador
  atômico de ID e status dos jobs assíncronos. Consome a fila do
  RabbitMQ para processar criações.
- **`proto`** — contrato gRPC/Protobuf compartilhado entre os dois
  serviços (`MovieService`: `GetMovie`, `ListMovie`, `CreateMovie`,
  `DeleteMovie`, `GetMovieStatus`).
- **`shared`** — DTOs compartilhados entre `api-gateway` e
  `movies-service` (a mensagem publicada na fila).

Os quatro módulos vivem num único `go.work` (workspace do Go), não em
repositórios separados.

### Por que criação é assíncrona

`POST /movies` não cria o filme na hora — publica uma mensagem na fila e
devolve `202 Accepted` com um `correlation_id` na hora. O `movies-service`
consome a fila, cria o filme e grava o resultado; o cliente confere o
resultado em `GET /movies/status/{correlationId}`. O porquê dessa decisão,
os bugs que ela teve no caminho (ID fixo, desserialização quebrada) e como
foram corrigidos: `docs/adr/0001-*.md`.

## Rodando localmente

```bash
cp .env.example .env
docker compose up -d --build
curl http://localhost:8080/health
```

Sobe 5 containers: `api-gateway`, `movies-service`, MongoDB (com seed
automático de ~28 mil filmes), RabbitMQ e Jaeger (UI de trace distribuído
em `http://localhost:16686`). Nenhuma dependência de nuvem — tudo roda na
sua máquina.

## API

Base path: `/api/v1`. Documentação interativa completa (Swagger) em
`http://localhost:8080/swagger/index.html` depois de subir a stack.

| Método | Rota | O que faz |
|---|---|---|
| `GET` | `/movies` | Lista filmes — filtro por `title`/`year`, paginação, ordenação |
| `GET` | `/movies/{id}` | Busca um filme por ID |
| `POST` | `/movies` | Enfileira a criação de um filme, devolve `correlation_id` |
| `GET` | `/movies/status/{correlationId}` | Consulta o resultado de uma criação assíncrona |
| `DELETE` | `/movies/{id}` | Remove um filme |
| `GET` | `/health` | Healthcheck |
| `GET` | `/metrics` | Métricas Prometheus (`api-gateway` apenas — ver "Observabilidade") |

## Testes

```bash
# Cada módulo é testado separadamente (workspace do Go, não um módulo só):
cd movies-service && go build ./... && go vet ./... && go test -p 1 ./...
cd api-gateway     && go build ./... && go vet ./... && go test ./...
```

Os testes de integração de `movies-service` (pacote `mongodb`, `seed`)
precisam de um MongoDB real em `localhost:27017` — sem ele, falham rápido
de propósito, em vez de mascarar infraestrutura quebrada com `t.Skip`
(decisão registrada em `docs/adr/0001-*.md`). CI (`.github/workflows/go.yml`)
já sobe esse MongoDB automaticamente.

## Performance

Testado com [vegeta](https://github.com/tsenart/vegeta) e uma ferramenta
própria (`docs/performance/tools/ingest-load`) contra o dataset real de
seed (28.451 filmes). **Ambiente: local** (`docker compose up`), não
produção — este projeto ainda não tem deploy público, ver
[`docs/performance/README.md`](docs/performance/README.md#próximos-passos).
Nenhum número abaixo foi estimado; todos vêm de uma rodada de carga real, e
metodologia + dados brutos completos (inclusive os erros de medição que
corrigi no caminho) estão documentados lá.

| Métrica | Valor | Teste |
|---|---|---|
| `GET /health` | 100 req/s, p99 1.1ms, 100% sucesso | `vegeta attack -rate=100 -duration=20s` |
| `GET /movies/{id}` (por `_id`) | 100 req/s, p99 3.0ms, 100% sucesso | idem |
| `GET /movies` (listagem, 28k docs) | 500 req/s, p99 5.9ms, 100% sucesso | idem, após corrigir índice ausente |
| `GET /movies/status/{id}` | 200 req/s, p99 2.5ms, 100% sucesso | idem |
| `DELETE /movies/{id}` | 100 req/s, p99 2.3ms, 100% sucesso | testado contra filmes descartáveis, não contra o dataset de seed |
| Criação assíncrona (`POST /movies` → RabbitMQ → `movies-service`) | 2.013 filmes/s sustentados com 4 consumers (escala sub-linear — ver ADR 0005) | publish direto no RabbitMQ + contagem no Mongo, 100k mensagens |
| Memória sob carga sustentada (60s) | sem crescimento contínuo em nenhum dos 4 containers | `docker stats` amostrado a cada 5s |

**Achado real:** a listagem não tinha índice em `title` (campo da
ordenação padrão) — em 80 req/s ela colapsava para uma latência média de
1.22s (p99 3.28s) com o MongoDB em 582% de CPU. Adicionar o índice trouxe
a mesma taxa para 1.45ms de latência média — sem mudar hardware nenhum
(decisão em `docs/adr/0004-*.md`). Antes/depois completo, com números de
todas as taxas testadas, em [`docs/performance/README.md`](docs/performance/README.md).

### Como reproduzir

```bash
docker compose up -d --build
go install github.com/tsenart/vegeta@latest   # ~/go/bin precisa estar no PATH

# Endpoints HTTP síncronos:
echo "GET http://localhost:8080/api/v1/movies?limit=10" > /tmp/target.txt
vegeta attack -targets=/tmp/target.txt -rate=500 -duration=15s | vegeta report

# Pipeline assíncrono de criação (publica N filmes e mede throughput real):
cd docs/performance/tools/ingest-load && go run . -url http://localhost:8080 -n 2000 -c 50 -drain=false
```

## Observabilidade

`api-gateway` expõe métricas Prometheus em `/metrics` (contagem e latência
por rota, sem cardinalidade não-limitada — usa o padrão de rota do Gin,
não a URL crua). Stack completo de Prometheus + Grafana rodando em
Kubernetes local: ver `infra/kubernetes/monitoring/`.

Além disso, os dois serviços empurram métricas para o
[Hermes](https://github.com/FranciscoHonorat/hermes-observability), uma
plataforma de observabilidade própria — modelo push, sem precisar de um
endpoint `/metrics` novo. É assim que `movies-service` (que só fala gRPC,
sem listener HTTP) tem métricas pela primeira vez:
`grpc_requests_total` / `grpc_request_duration_ms` / `grpc_errors_total`
via um interceptor gRPC dedicado (`packages/agent-go/grpcmetrics`).
`api-gateway` reporta ao Hermes em paralelo ao Prometheus —
`http_requests_total` / `http_request_duration_ms` / `http_errors_total`
— sem substituir nada do que já existia. Hermes cobre só métricas; trace
continua exclusivamente no Jaeger, para não duplicar spans. Detalhes,
inclusive por que o cliente Go do Hermes isola a dependência do gRPC num
módulo Go separado e como os containers alcançam o Collector do Hermes
(que roda numa stack Docker à parte) em
[`docs/adr/0008-*.md`](docs/adr/0008-metricas-hermes-alem-do-prometheus.md).

## Trace distribuído

Toda requisição gera uma trace OpenTelemetry que atravessa `api-gateway` →
`movies-service` → MongoDB — incluindo o caminho assíncrono via RabbitMQ,
onde o contexto de trace viaja nos headers AMQP da própria mensagem
(`shared.AMQPHeaderCarrier`, já que não existe instrumentação automática
pra fila de mensagens do jeito que existe pra HTTP/gRPC). Visualize em
`http://localhost:16686` (Jaeger) depois de `docker compose up`.

Confirmado contra o Jaeger real, não só o código: uma trace do caminho
síncrono (`GET /movies/{id}`) mostra `api-gateway` → chamada gRPC →
`movies-service` → consulta ao Mongo, tudo com relação pai-filho correta;
uma trace do caminho assíncrono (`POST /movies`) mostra a publicação no
RabbitMQ e o consumo do outro lado como parte da mesma trace. Detalhes,
inclusive uma incompatibilidade real que encontrei (a instrumentação
oficial de MongoDB não suporta a v2 do driver que este projeto usa) em
[`docs/adr/0007-*.md`](docs/adr/0007-trace-distribuido-com-jaeger.md).

## Resiliência

As duas chamadas de rede do `api-gateway` (gRPC para `movies-service`,
publish no RabbitMQ) passam por retry com backoff + circuit breaker
(`api-gateway/internal/resilience`) — uma dependência fora do ar vira uma
falha rápida e previsível (~8ms com o breaker aberto), não uma requisição
travada por dezenas de segundos. Estado de cada breaker é visível ao vivo
em `/metrics` (`circuit_breaker_state`, `circuit_breaker_trips_total`).

Validado contra o `docker-compose` real, não só com mocks — parar e
religar `movies-service`/`rabbitmq` revelou dois bugs reais no caminho
(uma chamada gRPC sem timeout que travava por 20s, e um publisher
RabbitMQ que nunca reconectava sozinho) documentados, com os números
medidos, em [`docs/adr/0006-*.md`](docs/adr/0006-circuit-breaker-e-retry-no-api-gateway.md).

## Infraestrutura e deploy

Kubernetes local (`kind`) e Terraform (contra LocalStack, sem custo real)
— tudo documentado em [`infra/README.md`](infra/README.md), incluindo o
porquê de cada desvio do template de infra original (sem EKS/RDS/ElastiCache,
por exemplo). Deploy público ainda não existe; ver
[`docs/performance/README.md`](docs/performance/README.md#próximos-passos)
para o estado dessa decisão.

## Documentação

Este README cobre o essencial para rodar e usar o projeto. O raciocínio
por trás de cada decisão não-óbvia mora em `docs/`:

- **`docs/adr/`** — Architecture Decision Records: o que foi decidido, as
  alternativas consideradas e por que foram descartadas, numeradas em
  ordem cronológica (0001 a 0008 até agora).
- **`docs/learning/`** — lições generalizáveis tiradas ao longo do
  caminho (não específicas deste projeto), com referências bibliográficas
  quando aplicável — ex.: por que uma tag de struct mal formatada pode ser
  um bug de disponibilidade, ou como um teste de carga mal desenhado pode
  medir a si mesmo em vez do sistema.
- **`docs/performance/`** — metodologia e resultados completos de todos
  os testes de carga, com os relatórios brutos.

## Licença

MIT — ver [`LICENSE`](LICENSE).
