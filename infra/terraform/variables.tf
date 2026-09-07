variable "project_name" {
  description = "Prefixo usado no nome de todo recurso criado, para diferenciar de outros projetos no mesmo bucket/conta."
  type        = string
  default     = "re-api-books"
}

variable "environment" {
  description = "Rótulo do ambiente (local, dev, staging, prod). \"local\" é o único usado neste projeto de estudo."
  type        = string
  default     = "local"
}

variable "aws_region" {
  description = "Região AWS (ou emulada pelo LocalStack)."
  type        = string
  default     = "us-east-1"
}

variable "use_localstack" {
  description = "true (padrão): aponta o provider para LocalStack, sem custo real. false: usa AWS de verdade — só faça isso conscientemente, ver infra/terraform/README.md."
  type        = bool
  default     = true
}

variable "localstack_endpoint" {
  description = "Endpoint do LocalStack. Só é usado quando use_localstack = true."
  type        = string
  default     = "http://localhost:4566"
}

variable "mongo_backup_retention_days" {
  description = "Dias de retenção dos backups do MongoDB no bucket S3 (ver infra/backup-recovery/backup-strategy.md)."
  type        = number
  default     = 14
}
