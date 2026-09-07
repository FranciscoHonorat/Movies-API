# Log group placeholder. Neither service ships logs to CloudWatch today
# (both just write to stdout, which is the right default for containers —
# see infra/monitoring/README.md); this exists so the pattern is here to
# wire up (e.g. via the CloudWatch agent or a Fluent Bit sidecar) if this
# project ever runs somewhere CloudWatch is the log sink of choice.
resource "aws_cloudwatch_log_group" "app" {
  name              = "/${var.project_name}/${var.environment}"
  retention_in_days = 14

  tags = {
    Name        = "${var.project_name}-logs"
    Environment = var.environment
  }
}
