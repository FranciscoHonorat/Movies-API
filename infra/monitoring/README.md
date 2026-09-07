# Monitoring — re-api-books

## Onde está o quê

- **Config de scrape do Prometheus e regras de alerta**: dentro de
  `infra/kubernetes/monitoring/prometheus.yaml` (um `ConfigMap`), não aqui.
  Manter um único lugar evita a versão desse arquivo divergir de uma cópia
  "standalone" em `infra/monitoring/` — o problema clássico de template
  genérico que lista `prometheus.yml` em dois diretórios diferentes ao
  mesmo tempo.
- **Dashboard do Grafana**: `grafana-dashboards/api-gateway.json` — importe
  manualmente em `http://localhost:3000` (depois de
  `kubectl port-forward svc/grafana-service 3000:3000 -n re-api-books-dev`)
  ou provisione via ConfigMap se preferir automatizar (fora do escopo desta
  adaptação inicial).
- **`sentry-config.js`** do template original: não incluído. Sentry é um
  serviço de terceiros pago acima de um tier free limitado — para um
  projeto de estudo sem custo real, os logs estruturados que já existem
  (`log/slog` no `api-gateway`) mais o Prometheus/Grafana aqui cobrem
  observabilidade sem depender de outra conta externa. Reavaliar se este
  projeto algum dia rodar em produção de verdade com usuários reais.

## O que está instrumentado

Só o `api-gateway` expõe métricas (`GET /metrics`, Prometheus text format),
via `api-gateway/internal/observability`: contagem de requisições
(`http_requests_total`, por método/rota/status) e latência
(`http_request_duration_seconds`, histograma por método/rota). O label de
rota usa o padrão do Gin (`c.FullPath()`, ex. `/movies/:id`), não a URL
crua — isso evita cardinalidade não-limitada (uma série por ID de filme
diferente).

`movies-service` não tem `/metrics` — ele só fala gRPC, e adicionar
Prometheus ali exigiria subir um listener HTTP à parte só para isso. Fica
para uma iteração futura (ver `docs/adr/0003-*.md`, "Itens descobertos").
Enquanto isso, o Prometheus só sabe se `movies-service` está de pé
indiretamente, através do healthcheck TCP do próprio Kubernetes
(`backend-movies-service-deployment.yaml`), não através de métricas.
