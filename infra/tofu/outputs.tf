output "kubeconfig_path" {
  description = "Kubeconfig do cluster kind (use com KUBECONFIG=... kubectl)."
  value       = kind_cluster.this.kubeconfig_path
}

output "namespace" {
  value = helm_release.movies_api.namespace
}

output "api_gateway_url" {
  description = "Como acessar o api-gateway."
  value       = var.ingress_enabled ? "http://movies-api.local (adicione '127.0.0.1 movies-api.local' no /etc/hosts)" : "kubectl -n ${var.namespace} port-forward svc/movies-api-api-gateway 8080"
}

output "mongo_root_password" {
  value     = random_password.mongo.result
  sensitive = true
}

output "rabbitmq_password" {
  value     = random_password.rabbitmq.result
  sensitive = true
}
