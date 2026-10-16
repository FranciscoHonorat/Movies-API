variable "cluster_name" {
  description = "Nome do cluster kind local."
  type        = string
  default     = "movies-api"
}

variable "kubernetes_version" {
  description = "Imagem do nó kind (define a versão do Kubernetes)."
  type        = string
  default     = "kindest/node:v1.36.4"
}

variable "namespace" {
  description = "Namespace onde o chart é instalado."
  type        = string
  default     = "movies-api"
}

variable "image_registry" {
  description = "Registry das imagens (ex.: ghcr.io/franciscohonorat). Vazio = imagens locais carregadas no kind."
  type        = string
  default     = ""
}

variable "image_tag" {
  description = "Tag das imagens do api-gateway e do movies-service."
  type        = string
  default     = "latest"
}

variable "load_local_images" {
  description = "Se true, roda `kind load docker-image` para as imagens locais antes do helm install."
  type        = bool
  default     = true
}

variable "ingress_enabled" {
  description = "Instala o ingress-nginx e expõe o api-gateway em http://movies-api.local (porta 80 do host)."
  type        = bool
  default     = false
}
