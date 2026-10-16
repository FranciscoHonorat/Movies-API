# Infra 100% local e sem custo: um cluster kind (Kubernetes dentro do
# Docker) + o chart Helm de infra/helm/movies-api. Ver docs/adr/0003.

resource "kind_cluster" "this" {
  name           = var.cluster_name
  node_image     = var.kubernetes_version
  wait_for_ready = true

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"

    node {
      role = "control-plane"

      # Rótulo que o ingress-nginx (provider kind) usa para escolher o nó.
      kubeadm_config_patches = [
        "kind: InitConfiguration\nnodeRegistration:\n  kubeletExtraArgs:\n    node-labels: \"ingress-ready=true\"\n"
      ]

      # Só ocupa as portas 80/443 do host quando o ingress está ligado.
      dynamic "extra_port_mappings" {
        for_each = var.ingress_enabled ? [80, 443] : []
        content {
          container_port = extra_port_mappings.value
          host_port      = extra_port_mappings.value
        }
      }
    }
  }
}

provider "helm" {
  kubernetes = {
    host                   = kind_cluster.this.endpoint
    client_certificate     = kind_cluster.this.client_certificate
    client_key             = kind_cluster.this.client_key
    cluster_ca_certificate = kind_cluster.this.cluster_ca_certificate
  }
}

resource "random_password" "mongo" {
  length  = 24
  special = false
}

resource "random_password" "rabbitmq" {
  length  = 24
  special = false
}

locals {
  images = ["api-gateway", "movies-service"]
}

# Imagens construídas localmente (docker build) não existem dentro do nó
# kind; `kind load` copia para lá. Desligue com load_local_images = false
# quando usar um registry.
resource "terraform_data" "load_images" {
  for_each = var.load_local_images && var.image_registry == "" ? toset(local.images) : toset([])

  triggers_replace = [kind_cluster.this.id, var.image_tag]

  provisioner "local-exec" {
    command = "kind load docker-image ${each.key}:${var.image_tag} --name ${kind_cluster.this.name}"
  }
}

resource "helm_release" "ingress_nginx" {
  count = var.ingress_enabled ? 1 : 0

  name             = "ingress-nginx"
  repository       = "https://kubernetes.github.io/ingress-nginx"
  chart            = "ingress-nginx"
  version          = "4.15.1"
  namespace        = "ingress-nginx"
  create_namespace = true
  wait             = true

  # Equivalente ao manifest "provider/kind" do ingress-nginx.
  values = [yamlencode({
    controller = {
      hostPort       = { enabled = true }
      service        = { type = "NodePort" }
      nodeSelector   = { "ingress-ready" = "true" }
      tolerations    = [{ key = "node-role.kubernetes.io/control-plane", operator = "Equal", effect = "NoSchedule" }]
      publishService = { enabled = false }
      extraArgs      = { "publish-status-address" = "localhost" }
    }
  })]
}

resource "helm_release" "movies_api" {
  name             = "movies-api"
  chart            = "${path.module}/../helm/movies-api"
  namespace        = var.namespace
  create_namespace = true
  wait             = true
  timeout          = 600

  values = [yamlencode({
    image = {
      registry   = var.image_registry
      tag        = var.image_tag
      pullPolicy = var.image_registry == "" ? "Never" : "IfNotPresent"
    }
    ingress = { enabled = var.ingress_enabled }
  })]

  set_sensitive = [
    { name = "mongodb.rootPassword", value = random_password.mongo.result },
    { name = "rabbitmq.password", value = random_password.rabbitmq.result },
  ]

  depends_on = [terraform_data.load_images, helm_release.ingress_nginx]
}
