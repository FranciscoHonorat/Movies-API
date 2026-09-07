# Terraform — re-api-books

Provisiona só o que este projeto realmente usa fora do cluster Kubernetes:
uma VPC (para aprendizado/paridade com o template original — nada roda
dentro dela hoje), um bucket S3 para backups do MongoDB, e um log group do
CloudWatch. **Sem EKS, RDS ou ElastiCache** — ver `docs/adr/0003-*.md` para
o porquê.

## Modo padrão: LocalStack, sem custo

```bash
# 1. Suba o LocalStack (emulador de AWS rodando em Docker, grátis, OSS):
docker run -d --name localstack -p 4566:4566 localstack/localstack

# 2. Init + apply, apontando para o LocalStack:
cd infra/terraform
terraform init
terraform plan  -var-file=environments/local.tfvars
terraform apply -var-file=environments/local.tfvars

# 3. Quando terminar:
terraform destroy -var-file=environments/local.tfvars
docker rm -f localstack
```

`use_localstack = true` (o padrão em `environments/local.tfvars`) faz o
provider AWS conversar com `http://localhost:4566` em vez da AWS de
verdade — `terraform apply` aqui nunca gera cobrança.

## Modo opt-in: AWS real

Só se você deliberadamente quiser recursos reais (ex.: testar o bucket S3
de backup contra o S3 de verdade). Custo real, ainda que pequeno para os
recursos deste diretório (bucket S3 + log group — não há EKS/RDS aqui para
gerar custo maior). Ver `environments/prod.tfvars.example`.

```bash
cp environments/prod.tfvars.example environments/prod.tfvars  # editar se necessário; já está no .gitignore
aws configure  # credenciais reais via AWS CLI/env vars, nunca em .tfvars
terraform apply -var-file=environments/prod.tfvars
```

## Validado neste ambiente?

Não — este ambiente de desenvolvimento não tem o binário `terraform`
instalado, então este HCL foi escrito com cuidado mas **não passou por
`terraform validate`/`plan` reais**. Rode `terraform validate` antes do
primeiro `apply`.
