# Infraestrutura — re-api-books

Adaptação do template de infra genérico (Terraform + EKS + RDS +
ElastiCache + S3 + CloudWatch + Kubernetes + CI/CD + monitoring + security
+ backup/DR) para este projeto especificamente. **Não** é uma cópia
1-para-1 — várias peças do template não fazem sentido aqui, e a decisão de
cada desvio está registrada, com o raciocínio, em `docs/adr/0003-*.md`
(decisão) e `docs/learning/0002-*.md` (lições generalizáveis, com
referências bibliográficas).

**Regra que guiou toda a adaptação:** este é um projeto de estudo — nada
aqui pode gerar custo real por padrão. Ver "Custo" abaixo.

## Estrutura

```
infra/
├── README.md                 # este arquivo
├── terraform/                 # VPC + S3 (backups) + CloudWatch, via LocalStack por padrão
│   └── environments/
├── kubernetes/                 # cluster local (kind), sem Helm/Kustomize
│   ├── namespace.yaml           # dev/staging/prod + Pod Security Admission
│   ├── configmap.yaml / secrets.yaml.example
│   ├── backend-*.yaml           # api-gateway, movies-service
│   ├── mongodb-*.yaml / rabbitmq-*.yaml
│   ├── backend-hpa.yaml / ingress.yaml / kind-config.yaml
│   └── monitoring/              # Prometheus + Grafana, in-cluster
├── cicd/
│   └── scripts/                 # build.sh, test.sh, deploy.sh
├── monitoring/
│   └── grafana-dashboards/
├── security/
│   └── network-policies.yaml
├── backup-recovery/
│   └── mongodb-backup.sh, restore-procedure.md, backup-strategy.md
└── local-dev/
    └── README.md                 # aponta para o docker-compose.yml da raiz, não duplica
```

## O que o template original tinha e foi deliberadamente omitido/desviado

| Item do template | Decisão aqui | Por quê (resumo — detalhe na ADR) |
|---|---|---|
| `terraform/eks.tf` | omitido | EKS cobra por hora de controle mesmo ocioso; usa `kind` local (grátis) em vez disso |
| `terraform/rds.tf` | omitido | projeto não tem banco relacional (é MongoDB, self-hosted) |
| `terraform/elasticache.tf` | omitido | projeto não usa cache/Redis |
| `cicd/.github/workflows/` | movido para a raiz `.github/workflows/` | GitHub Actions só lê workflows em `.github/workflows/` na raiz — não há como relocar |
| `docker/*.dockerfile` | mantido onde já estava (`movies-service/Dockerfile`, `api-gateway/Dockerfile`) | Dockerfile ao lado do serviço que builda é o padrão idiomático de monorepo Go; mover exigiria reescrever paths de build sem ganho |
| `kubernetes/secrets.yaml` | vira `secrets.yaml.example` (template) | nunca commitar segredo real; ver `.gitignore` |
| `monitoring/sentry-config.js` | omitido | serviço de terceiros pago acima do tier free; logs estruturados + Prometheus/Grafana cobrem o essencial sem conta externa |
| `security/falco-rules.yaml` | omitido | DaemonSet privilegiado pesado demais para 2 serviços de estudo — ver `infra/security/README.md` |
| `security/pod-security-policies.yaml` | vira labels de Pod Security Admission | PodSecurityPolicy foi removido do Kubernetes na 1.25; um manifesto desse tipo simplesmente falha hoje |
| `security/ssl-tls/` | omitido | sem domínio público neste projeto; `cert-manager` é o caminho certo se isso mudar |
| `backup-recovery/redis-backup.sh` | omitido | sem Redis neste projeto |
| `local-dev/docker-compose.yml` | mantido na raiz, não duplicado | já é a fonte de verdade usada por `docker-image.yml`; ver `infra/local-dev/README.md` |

## Custo

Tudo aqui roda de graça por padrão:

- **Kubernetes**: `kind` (cluster em Docker, local).
- **Terraform**: aponta para **LocalStack** (emulador de AWS em Docker) por
  padrão — `use_localstack = true` em `environments/local.tfvars`. Só gera
  custo real se você deliberadamente copiar
  `environments/prod.tfvars.example` para `prod.tfvars` e aplicar contra
  AWS de verdade (ver `infra/terraform/README.md`).
- **Monitoring**: Prometheus + Grafana OSS, rodando como pods no próprio
  cluster local.
- **CI**: GitHub Actions, dentro do tier gratuito de um repositório
  público/pessoal.

## Deploy

Ver `DEPLOYMENT.md` para o passo a passo completo, ou direto:

```bash
infra/cicd/scripts/deploy.sh
```
