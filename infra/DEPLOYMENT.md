# Deployment passo a passo — re-api-books

Público-alvo: você, daqui a alguns meses, tendo esquecido a ordem exata.
Tudo aqui é local e gratuito — ver `infra/README.md`, seção "Custo".

## Pré-requisitos

- Docker
- [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- `kubectl`
- (opcional) [`terraform`](https://developer.hashicorp.com/terraform/install) — só se você for usar `infra/terraform/`
- (opcional) AWS CLI — só se `infra/backup-recovery/mongodb-backup.sh` for enviar backups para o bucket S3/LocalStack

## 1. Infraestrutura "de nuvem" (opcional, LocalStack)

Pule esta seção se você só quer rodar a aplicação — ela não depende de
nada aqui. Faça isso só se quiser exercitar o bucket S3 de backup ou o
log group do CloudWatch, mesmo que emulados.

```bash
docker run -d --name localstack -p 4566:4566 localstack/localstack
cd infra/terraform
terraform init
terraform apply -var-file=environments/local.tfvars
cd -
```

## 2. Cluster Kubernetes local

```bash
kind create cluster --name re-api-books --config infra/kubernetes/kind-config.yaml
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl patch deployment metrics-server -n kube-system --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.13.0/deploy/static/provider/kind/deploy.yaml
kubectl wait --namespace ingress-nginx --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller --timeout=120s
```

(`infra/cicd/scripts/deploy.sh` já faz cluster + metrics-server + ingress-nginx
automaticamente na primeira vez — os comandos acima são o que ele roda por
baixo, para quando você quiser fazer manualmente ou entender uma falha.)

## 3. Segredos

```bash
cp infra/kubernetes/secrets.yaml.example infra/kubernetes/secrets.yaml
# edite infra/kubernetes/secrets.yaml com senhas (mesmo que só locais)
```

## 4. Deploy

```bash
infra/cicd/scripts/deploy.sh
```

Builda as duas imagens, carrega no `kind` (sem registry), aplica todo
`infra/kubernetes/*.yaml` + `infra/kubernetes/monitoring/*.yaml` +
`infra/security/network-policies.yaml`.

## 5. Validar

```bash
kubectl -n re-api-books-dev get pods
# todos "Running" e "READY 1/1" antes de seguir

# api-gateway, via ingress:
echo "127.0.0.1 re-api-books.local" | sudo tee -a /etc/hosts
curl http://re-api-books.local/health

# ou sem ingress, via port-forward:
kubectl -n re-api-books-dev port-forward svc/api-gateway-service 8080:8080
curl http://localhost:8080/health

# métricas do api-gateway:
curl http://localhost:8080/metrics | head

# Grafana:
kubectl -n re-api-books-dev port-forward svc/grafana-service 3000:3000
# abrir http://localhost:3000 (admin/admin), importar
# infra/monitoring/grafana-dashboards/api-gateway.json
```

## 6. Desmontar

```bash
kind delete cluster --name re-api-books
docker rm -f localstack   # se você tiver feito o passo 1
```
