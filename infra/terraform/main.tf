# Provider pinned to LocalStack (https://localstack.cloud) by default —
# `terraform apply` against this config talks to a Docker container on your
# own machine, not to real AWS, so it never incurs cloud cost. See
# infra/terraform/README.md for how LocalStack is started and for the
# (opt-in, explicitly not-free) path to pointing this at real AWS instead.
#
# There is deliberately no eks.tf / rds.tf / elasticache.tf here, unlike the
# original infra/ template this was adapted from — see docs/adr/0003 for
# why: this project runs Kubernetes via a local `kind` cluster (not
# Terraform-managed EKS, which bills per hour whether or not anything is
# running on it) and has no relational database or cache layer to
# provision (MongoDB and RabbitMQ are self-hosted, deployed via
# infra/kubernetes/, not as managed AWS services).

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  # Dummy credentials: LocalStack doesn't check them, but the AWS provider
  # refuses to start without *something* set. Never used against real AWS —
  # environments/prod.tfvars.example (gitignored, opt-in only) documents
  # real credentials come from environment variables instead, never from a
  # committed .tfvars file.
  access_key = var.use_localstack ? "test" : null
  secret_key = var.use_localstack ? "test" : null

  s3_use_path_style           = var.use_localstack
  skip_credentials_validation = var.use_localstack
  skip_metadata_api_check     = var.use_localstack
  skip_requesting_account_id  = var.use_localstack

  dynamic "endpoints" {
    for_each = var.use_localstack ? [1] : []
    content {
      s3         = var.localstack_endpoint
      ec2        = var.localstack_endpoint
      logs       = var.localstack_endpoint
      cloudwatch = var.localstack_endpoint
      sts        = var.localstack_endpoint
      iam        = var.localstack_endpoint
    }
  }
}
