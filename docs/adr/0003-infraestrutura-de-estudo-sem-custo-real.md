# ADR 0003: Adaptação de um template de infraestrutura AWS/EKS para um projeto de estudo sem custo real

- **Status:** Implementado
- **Data:** 2026-09-07
- **Escopo:** infraestrutura (`infra/`), fora do roadmap de auditoria de backend das ADRs 0001/0002

## Contexto

eu adaptei um template de árvore de diretórios `infra/` que usa
em outros projetos meus, pensado para uma stack AWS gerenciada: Terraform
provisionando VPC, EKS (Kubernetes gerenciado), RDS (banco relacional),
ElastiCache (Redis), S3 e CloudWatch; manifests Kubernetes com HPA, Ingress
e um stack de observabilidade (Prometheus/Grafana); pipelines de CI/CD sob
`infra/cicd/.github/workflows/`; configuração de segurança (Falco,
NetworkPolicy, PodSecurityPolicy, TLS/ACME); backup/DR para MongoDB e
Redis; e um `local-dev/docker-compose.yml`. Ele pediu para eu copiar esse
modelo para "todos os outros projetos" dele, adaptando-o primeiro para o
re-api-books.

Percebi duas coisas que tornavam impossível uma cópia literal:

1. **Descompasso de stack.** re-api-books não tem banco relacional (é
   MongoDB) nem cache (não usa Redis) — `rds.tf`/`elasticache.tf` não têm o
   que provisionar. RabbitMQ e MongoDB já rodam self-hosted via
   docker-compose e (agora) Kubernetes; nada neste projeto usa nenhum
   serviço gerenciado da AWS hoje.
2. **Restrição explícita do usuário: "projeto de estudos... não deve ter
   custos reais."** EKS cobra por hora de control plane mesmo com o
   cluster ocioso (e nós EC2 por cima disso); RDS/ElastiCache têm custo
   por hora semelhante. Provisionar qualquer um desses por padrão
   contradiz a restrição, mesmo que fosse "só para aprender o padrão".

Antes de começar, perguntei ao usuário e ele confirmou: manter o escopo só
neste projeto por ora (generalizar para os outros projetos fica para
quando ele pedir, projeto a projeto), e priorizar zero custo sobre
fidelidade ao template original.

## Decisão

1. **Optei por Kubernetes local (`kind`), não EKS.** Não coloquei nenhum
   `eks.tf` em `infra/kubernetes/` — o "cluster gerenciado" do template
   virou, na minha adaptação, um cluster `kind` (Kubernetes-em-Docker,
   gratuito, o fluxo mais popular em material de aprendizado de
   Kubernetes). Os manifests que já existiam em `k8s/` (que migrei para
   `infra/kubernetes/`, ver item 6) já assumiam isso implicitamente
   (`imagePullPolicy: Never`, que só faz sentido sem um registry remoto) —
   minha decisão só formaliza algo que já era verdade na prática.
2. **Fiz o Terraform apontar para LocalStack por padrão, não AWS real.**
   Em `infra/terraform/main.tf` uso `use_localstack = true` como default
   (`environments/local.tfvars`), redirecionando o provider AWS para
   `http://localhost:4566` (um container Docker do LocalStack). Provisiono só
   o que o projeto de fato usaria se algum dia tocasse infraestrutura de
   nuvem: VPC (aprendizado/paridade, nada roda dentro dela hoje), um
   bucket S3 (destino opcional de `mongodb-backup.sh`) e um log group do
   CloudWatch (não usado ainda, mas deixei o padrão pronto). **Não escrevi
   `eks.tf`, `rds.tf` nem `elasticache.tf`** — não têm o que provisionar
   neste projeto. Deixei a AWS real disponível como opt-in explícito
   (`environments/prod.tfvars.example`, com aviso de custo real e já no
   `.gitignore`), nunca como padrão.
3. **Escolhi observabilidade in-cluster, sem SaaS pago.** Coloquei
   Prometheus + Grafana OSS como `Deployment`s dentro do próprio cluster
   (`infra/kubernetes/monitoring/`), no lugar do `sentry-config.js` do
   template (Sentry é pago acima de um tier free). Também dei ao
   `api-gateway` um endpoint `/metrics` de verdade
   (`api-gateway/internal/observability/metrics.go`, usando
   `prometheus/client_golang`) — sem isso, o Prometheus não teria nada
   para coletar; deixei `movies-service` sem métricas por ora (só fala
   gRPC hoje), registrado como pendência.
4. **Optei por segurança com o que já vem no Kubernetes, não com add-ons
   pesados.** Usei `NetworkPolicy` (built-in) no lugar de exigir um
   service mesh, e labels de Pod Security Admission (built-in desde o
   Kubernetes 1.25) no lugar de `PodSecurityPolicy` — que foi
   **removido** do Kubernetes na 1.25 e simplesmente não funcionaria se eu
   tivesse copiado do template como estava. Deixei Falco e TLS/ACME
   (`cert-manager`) de fora — ver "Alternativas consideradas".
5. **Coloquei scripts reutilizáveis em `infra/cicd/scripts/`, e deixei os
   workflows do GitHub na raiz.** `infra/cicd/.github/workflows/` do
   template não é fisicamente possível com GitHub Actions — o GitHub só
   descobre workflows em `.github/workflows/` na raiz do repositório. Em
   vez de fingir que essa pasta funciona, coloquei
   `build.sh`/`test.sh`/`deploy.sh` em `infra/cicd/scripts/` como lógica
   reutilizável local, e deixei os workflows de fato (`go.yml`,
   `docker-image.yml`, já existentes desde antes desta ADR) onde o GitHub
   exige.
6. **Migrei `k8s/` para `infra/kubernetes/`** (`git mv`, não cópia) —
   para manter uma fonte única de verdade. Aproveitei a mudança para dar
   aos manifests existentes: namespace dedicado (`re-api-books-dev`, mais
   `-staging`/`-prod` como estrutura para o futuro), credenciais vindas de
   um `Secret` (`re-api-books-secrets`) em vez de hardcoded em texto
   plano no próprio `Deployment` (como estavam — `root:password`,
   `guest:guest` diretamente no YAML, committado),
   `resources.requests/limits`, `readinessProbe`/`livenessProbe`, e
   corrigi um `imagePullPolicy: Never` que tinha sido copiado por engano
   para a imagem pública `rabbitmq:3-management` (o que travaria o pod em
   `ImagePullBackOff` em qualquer cluster sem essa imagem já em cache
   local).
7. **Não dupliquei o que já existia em `local-dev/` e `docker/`.** Deixei
   o `docker-compose.yml` na raiz (é o que `docker-image.yml` já usa) e os
   `Dockerfile`s ao lado de cada serviço (padrão idiomático de monorepo
   Go, e é o path que `docker-compose.yml` já referencia).
   `infra/local-dev/README.md` só aponta para eles.

## Consequências

**Positivas**
- Todo o `infra/` roda sem custo de nuvem por padrão — validei isso
  estruturalmente (nenhum recurso Terraform sem `use_localstack` como
  guarda, nenhum passo de deploy que dependa de um cluster gerenciado).
- Ao migrar `k8s/` para `infra/kubernetes/`, corrigi de brinde credenciais
  em texto plano committadas e um `imagePullPolicy` errado — bugs que já
  existiam antes desta ADR e que só encontrei por causa da revisão que a
  adaptação me forçou a fazer.
- `api-gateway` ganha sua primeira métrica de verdade (`/metrics`), não só
  um scrape config apontando para nada.

**Negativas / pendências que assumi conscientemente**
- Não validei o Terraform com `terraform validate`/`plan` reais — o
  binário não está disponível neste ambiente de desenvolvimento (ver
  `infra/terraform/README.md`, seção "Validado neste ambiente?").
- `movies-service` continua sem métricas Prometheus (só gRPC, sem
  listener HTTP dedicado).
- `NetworkPolicy` não tem efeito no CNI padrão do `kind` (`kindnet`) — só
  funciona de fato com Calico, o que documentei mas não configurei por
  padrão (achei o custo de complexidade maior que o benefício de validar
  policies num cluster descartável de estudo).
- Os `Deployment`s não definem `securityContext`; deixei o namespace
  `-prod` (nunca de fato usado por `deploy.sh`) com Pod Security Admission
  em modo `restricted`, o que rejeitaria esses mesmos manifests se alguém
  tentasse aplicá-los lá hoje. Documentei isso em
  `infra/kubernetes/README.md`, não escondi.

## Alternativas consideradas

- **Provisionar EKS/RDS/ElastiCache reais mesmo assim, só para fins de
  aprendizado, e destruir logo depois:** rejeitei essa opção — mesmo um
  `apply` de minutos gera cobrança (EKS cobra o control plane por hora
  rodando, não por uso), e contraria a restrição explícita do usuário.
  Prefiro LocalStack para os primitivos AWS + `kind` para Kubernetes, que
  ensinam os mesmos padrões de Terraform/K8s sem esse risco.
- **k3d/minikube no lugar de kind:** equivalentes em custo (grátis,
  local). Escolhi `kind` só porque os manifests que já existiam
  (`imagePullPolicy: Never`) já indicavam que o projeto historicamente
  rodava contra um cluster local construído com imagens locais — o padrão
  mais comum para isso é `kind load docker-image`, que não tem
  equivalente direto tão simples no minikube (usaria `minikube image
  load` — funciona igual, mas trocar sem motivo custaria reescrever
  documentação/scripts à toa).
- **Falco mesmo assim, por ser gratuito:** rejeitei por ora — é grátis em
  custo de nuvem, mas caro em custo de operação (tuning de regras,
  DaemonSet privilegiado) para um cluster de estudo com 2 serviços. Vale
  reconsiderar se o projeto ganhar tráfego real.
- **Manter `infra/cicd/.github/workflows/` como no template, só por
  fidelidade, mesmo sabendo que o GitHub não lê de lá:** rejeitei — acho
  que manter um diretório que parece configuração real mas não faz nada é
  pior do que não tê-lo; confundiria uma futura leitura do repositório.

## Validação

- Confirmei `go build ./...`, `go vet ./...` e `go test ./...` limpos nos
  4 módulos Go do workspace, incluindo o novo pacote
  `api-gateway/internal/observability` (o teste cobre que o label de rota
  usa o padrão do Gin, não a URL crua — evita cardinalidade não-limitada
  nas métricas).
- Validei todo YAML sob `infra/` (mais `.github/workflows/*.yml` e
  `docker-compose.yml`) com `yaml.safe_load` — sintaticamente correto.
- Validei `infra/monitoring/grafana-dashboards/api-gateway.json` com
  `json.load`.
- Validei `infra/cicd/scripts/*.sh` e
  `infra/backup-recovery/mongodb-backup.sh` com `bash -n` (checagem de
  sintaxe, sem executar).
- **Não** validei o HCL sob `infra/terraform/` com `terraform validate`
  (o binário não estava disponível neste ambiente) — só fiz checagem
  manual de balanceamento de chaves/parênteses. Rodar `terraform validate`
  antes do primeiro uso real fica como responsabilidade de quem for
  aplicar.

## Itens descobertos durante a implementação, fora do escopo desta ADR

- `movies-service` sem métricas Prometheus (precisa de um listener HTTP
  dedicado, já que hoje só fala gRPC) e sem o protocolo padrão de
  gRPC health-check (`grpc.health.v1.Health`) — deixei o probe do
  Kubernetes usando só TCP connect por enquanto
  (`infra/kubernetes/backend-movies-service-deployment.yaml`).
- `NetworkPolicy` sem efeito real no CNI padrão do `kind` — funcional só
  com Calico, que não configurei por padrão.
- `securityContext` ausente em todos os `Deployment`s — bloquearia o
  namespace `-prod` (`enforce: restricted`) se algum dia for usado de
  verdade.
- A regra de `NetworkPolicy` de ingress para `api-gateway` ficou mais
  permissiva do que o ideal (`namespaceSelector: {}` libera qualquer
  namespace, não só `ingress-nginx`/`re-api-books-dev`) — simplifiquei
  deliberadamente por não haver um label de namespace padronizado para
  restringir com precisão sem mais configuração.
