# re-api-books

API de filmes em Go: `api-gateway` (REST, Gin) na frente de `movies-service`
(gRPC, MongoDB), com criação assíncrona via RabbitMQ. Projeto de estudo —
ver `docs/adr/` para o histórico de decisões e `docs/learning/` para as
lições tiradas ao longo do caminho.

## Rodando localmente

```bash
cp .env.example .env
docker compose up -d --build
curl http://localhost:8080/health
```

Swagger em `http://localhost:8080/swagger/index.html`. Deploy em
Kubernetes local (`kind`) e infraestrutura como código: ver `infra/README.md`.

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
a mesma taxa para 1.45ms de latência média — sem mudar hardware nenhum.
Antes/depois completo, com números de todas as taxas testadas, em
[`docs/performance/README.md`](docs/performance/README.md).

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
