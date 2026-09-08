# ADR 0008: Métricas via Hermes, além do Prometheus — e a primeira métrica do movies-service

- **Status:** Implementado
- **Data:** 2026-09-07
- **Escopo:** `api-gateway`, `movies-service`, `docker-compose.yml`, `.env.example`

## Contexto

`api-gateway` já expõe métricas Prometheus (`internal/observability/metrics.go`,
scrape em `/metrics`) e ambos os serviços já mandam traces pro Jaeger
(docs/adr/0007-*.md). `movies-service`, porém, nunca teve métrica nenhuma —
o comentário no topo de `metrics.go` já registrava o motivo: ele só serve
gRPC, e dar a ele métricas Prometheus significaria levantar um segundo
listener HTTP só pra expor `/metrics`, o que esse projeto nunca fez.

Comecei a usar o [Hermes](https://github.com/FranciscoHonorat/hermes-observability),
uma plataforma de observabilidade própria, em outro projeto, e resolvi
integrá-la aqui também — não pra substituir Prometheus/Grafana/Jaeger, que
continuam exatamente como estão, mas porque o modelo push do Hermes resolve
o buraco do `movies-service` de graça: o processo empurra métricas pro
Collector periodicamente, sem precisar de nenhum endpoint HTTP novo.

## Decisão

### 1. Aditivo, não substituto — e só métricas

Hermes entra lado a lado com o que já existe, sem tocar em nada:
Prometheus continua raspando `/metrics` do `api-gateway`, e o Jaeger
continua recebendo os spans dos dois serviços via OTLP. Não usei o
tracing do Hermes — rodar dois sistemas de trace ao mesmo tempo seria
redundante, sem nada a mais pra mostrar. Só métricas.

### 2. `google.golang.org/grpc` isolado num módulo Go separado (`grpcmetrics`)

O cliente Go do Hermes (`packages/agent-go`) não tem nenhuma dependência
externa — propriedade que eu queria preservar. Em vez de importar
`google.golang.org/grpc` no módulo principal só pra dar um interceptor de
métricas ao `movies-service`, criei `packages/agent-go/grpcmetrics` como
módulo Go próprio (mesmo padrão que o OpenTelemetry usa pra separar
`otelgrpc`/`otelgin` do SDK core). Só quem realmente serve gRPC paga o
custo dessa dependência.

`grpc.NewServer` aceita `StatsHandler` e `ChainUnaryInterceptor` ao mesmo
tempo, então o interceptor do Hermes convive sem atrito com o
`otelgrpc.NewServerHandler()` que já estava lá.

### 3. Nomes de métrica: os do Hermes, não os do Prometheus

`api-gateway` grava `http_requests_total` / `http_request_duration_ms` no
Hermes (não `http_request_duration_seconds`, como no Prometheus) e
`movies-service` grava `grpc_requests_total` / `grpc_request_duration_ms` /
`grpc_errors_total`. Escolha deliberada: são exatamente os nomes que o
dashboard embutido do Hermes já espera, então os dois serviços aparecem lá
sem precisar tocar em nenhuma UI.

### 4. Alcançar o Collector do host a partir da `movies-network`

O Collector do Hermes roda num docker-compose totalmente separado, numa
rede `hermes-network` que este projeto não enxerga — só está publicado no
host, na porta `4000`. Adicionei `extra_hosts: ["host.docker.internal:host-gateway"]`
em `api-gateway` e `movies-service`, e `HERMES_COLLECTOR_URL=http://host.docker.internal:4000`
como padrão — a forma portátil documentada pelo Docker de alcançar uma
porta publicada no host, em vez de fixar o IP do gateway da bridge (que
varia por máquina/versão do Docker).

### 5. Dependência via `replace` local, não publicada

`packages/agent-go` (e seu submódulo `grpcmetrics`) não estão publicados
em nenhum registro — são referenciados por `replace` apontando pro
caminho absoluto do checkout do Hermes nesta máquina, o mesmo padrão que
o próprio README do cliente Go documenta.

## Consequências

- `movies-service` tem visibilidade de métricas pela primeira vez, sem
  precisar de um listener HTTP novo.
- Duas fontes de métrica agora coexistem no `api-gateway` (Prometheus +
  Hermes) — aceitável aqui porque servem propósitos diferentes (scrape
  local vs. um dashboard central que também recebe o `movies-service`),
  mas vale reavaliar se algum dia isso virar confusão sobre qual métrica é
  "a fonte da verdade".
- O `replace` de caminho absoluto só funciona nesta máquina — se o projeto
  for rodar em CI ou em outra máquina sem o checkout do Hermes ao lado,
  isso precisa virar uma dependência publicada de verdade (ou o `replace`
  vira condicional/removido).
