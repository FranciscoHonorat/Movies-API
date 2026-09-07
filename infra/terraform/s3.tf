# Backup destination for infra/backup-recovery/mongodb-backup.sh
# (BACKUP_S3_BUCKET). Against LocalStack this is a bucket in the local
# emulator, not a real S3 bucket — free either way, but "free" here means
# "not billed", not "not really an S3 API": mongodb-backup.sh's `aws s3 cp`
# call is unchanged whether it's talking to LocalStack or real AWS.
resource "aws_s3_bucket" "mongo_backups" {
  bucket = "${var.project_name}-${var.environment}-mongo-backups"

  tags = {
    Name        = "${var.project_name}-mongo-backups"
    Environment = var.environment
  }
}

resource "aws_s3_bucket_versioning" "mongo_backups" {
  bucket = aws_s3_bucket.mongo_backups.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "mongo_backups" {
  bucket = aws_s3_bucket.mongo_backups.id

  rule {
    id     = "expire-old-backups"
    status = "Enabled"

    expiration {
      days = var.mongo_backup_retention_days
    }
  }
}

resource "aws_s3_bucket_public_access_block" "mongo_backups" {
  bucket = aws_s3_bucket.mongo_backups.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
