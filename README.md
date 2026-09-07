# re-api-books

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

Sobe 4 containers: `api-gateway`, `movies-service`, MongoDB (com seed
automático de ~28 mil filmes) e RabbitMQ. Nenhuma dependência de nuvem —
tudo roda na sua máquina.

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
| Criação assíncrona (`POST /movies` → RabbitMQ → `movies-service`) | ~1.400 filmes/s sustentados | contagem direta no Mongo durante um lote de 20.000 |
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
não a URL crua). `movies-service` ainda não expõe métricas (só fala gRPC
hoje). Stack completo de Prometheus + Grafana rodando em Kubernetes local:
ver `infra/kubernetes/monitoring/`.

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
  ordem cronológica (0001 a 0004 até agora).
- **`docs/learning/`** — lições generalizáveis tiradas ao longo do
  caminho (não específicas deste projeto), com referências bibliográficas
  quando aplicável — ex.: por que uma tag de struct mal formatada pode ser
  um bug de disponibilidade, ou como um teste de carga mal desenhado pode
  medir a si mesmo em vez do sistema.
- **`docs/performance/`** — metodologia e resultados completos de todos
  os testes de carga, com os relatórios brutos.

## Licença

MIT — ver [`LICENSE`](LICENSE).
