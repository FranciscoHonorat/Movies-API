# Infra

Tudo local e sem custo real (ver `docs/adr/0003`): um cluster **kind**
(Kubernetes dentro do Docker) e um chart **Helm**, ambos criados pelo
**OpenTofu**.

```
infra/
├── helm/movies-api/   # chart: api-gateway, movies-service, MongoDB, RabbitMQ, Jaeger, Ingress (opcional)
└── tofu/              # OpenTofu: cluster kind + senhas aleatórias + helm_release do chart
```

## Pré-requisitos

`docker`, `kind`, `helm`, `tofu` (OpenTofu ≥ 1.8) e `kubectl`.

## Subir tudo

```bash
# 1. Build das imagens com os nomes que o chart espera
H=/mnt/novovolume/arquivos/hermes-observability/packages/agent-go
docker build -t api-gateway:latest    -f api-gateway/Dockerfile    --build-context hermes-agent-go=$H .
docker build -t movies-service:latest -f movies-service/Dockerfile --build-context hermes-agent-go=$H .

# 2. Cluster + deploy
cd infra/tofu
tofu init
tofu apply          # cria o cluster kind, carrega as imagens e instala o chart

# 3. Usar
export KUBECONFIG=$(tofu output -raw kubeconfig_path)
kubectl -n movies-api get pods
kubectl -n movies-api port-forward svc/movies-api-api-gateway 8080
curl localhost:8080/health
```

Para usar as imagens publicadas pelo CI no GHCR em vez de buildar local,
copie `terraform.tfvars.example` para `terraform.tfvars` e defina
`image_registry = "ghcr.io/franciscohonorat"` e `load_local_images = false`
(os pacotes do GHCR precisam estar públicos).

Com `ingress_enabled = true` o OpenTofu também instala o ingress-nginx e o
api-gateway fica em `http://movies-api.local` (adicione
`127.0.0.1 movies-api.local` no `/etc/hosts`).

Derrubar tudo: `tofu destroy` (apaga o cluster kind inteiro).

## Só o chart (sem OpenTofu)

```bash
helm upgrade --install movies-api infra/helm/movies-api \
  -n movies-api --create-namespace \
  --set mongodb.rootPassword=troque --set rabbitmq.password=troque
```

`mongodb.rootPassword` e `rabbitmq.password` são obrigatórios: o chart
falha no render se estiverem vazios, para nunca subir com senha padrão.
Os demais valores estão comentados em `helm/movies-api/values.yaml`.

## CI

`.github/workflows/ci.yml` roda, em todo PR e push na `main`:

| Job    | O que faz |
|--------|-----------|
| lint   | `golangci-lint` (config em `/.golangci.yml`) em cada módulo |
| test   | `go build` + `go test` (com MongoDB como service container) |
| infra  | `helm lint`/`helm template` e `tofu fmt -check`/`tofu validate` |
| docker | build das duas imagens; no push na `main`, publica no GHCR (`:sha-…` e `:latest`) |

O repositório não versiona o `go.work` e depende do Hermes agent-go via
`replace` para um caminho absoluto; a action
`.github/actions/setup-workspace` recria os dois no runner.
