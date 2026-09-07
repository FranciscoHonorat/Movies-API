output "vpc_id" {
  value = aws_vpc.main.id
}

output "public_subnet_ids" {
  value = aws_subnet.public[*].id
}

output "mongo_backups_bucket" {
  description = "Passe este valor como BACKUP_S3_BUCKET para infra/backup-recovery/mongodb-backup.sh."
  value       = aws_s3_bucket.mongo_backups.bucket
}

output "app_log_group" {
  value = aws_cloudwatch_log_group.app.name
}
