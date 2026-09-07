# Default environment for this study project. Zero real cost: everything
# is applied against LocalStack (see infra/terraform/README.md).
project_name         = "re-api-books"
environment          = "local"
aws_region           = "us-east-1"
use_localstack       = true
localstack_endpoint  = "http://localhost:4566"
mongo_backup_retention_days = 14
