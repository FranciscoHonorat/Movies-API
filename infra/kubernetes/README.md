# Kubernetes — re-api-books

Manifests puros (`kubectl apply -f`, sem Helm/Kustomize — não valem a pena
para 2 serviços). Tudo assume um cluster **local** (`kind`), não um cluster
gerenciado na nuvem — ver `docs/adr/0003-*.md` para o porquê.

## Primeira vez, do zero

```bash
# 1. Cluster local (grátis, roda em Docker):
kind create cluster --name re-api-books --config kind-config.yaml

# 2. metrics-server — kind não vem com ele, mas backend-hpa.yaml precisa:
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
# em kind especificamente, o metrics-server precisa de --kubelet-insecure-tls
# (certificado do kubelet não é confiável pela CA padrão em clusters locais):
kubectl patch deployment metrics-server -n kube-system --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'

# 3. ingress-nginx (para ingress.yaml):
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.13.0/deploy/static/provider/kind/deploy.yaml

# 4. Secret com credenciais (nunca commitado — copie o template):
cp secrets.yaml.example secrets.yaml
# edite secrets.yaml com senhas reais (mesmo que "reais" só localmente)

# 5. Tudo de uma vez, ou passo a passo com kubectl apply -f <arquivo> -n re-api-books-dev:
../cicd/scripts/deploy.sh
```

Depois disso, `../cicd/scripts/deploy.sh` sozinho já cobre builda+carrega
imagem+aplica manifests para iterações seguintes (ele pula os passos 1–3 se
o cluster já existir).

## Sobre `infra/security/network-policies.yaml` e o CNI do kind

O CNI padrão do `kind` (`kindnet`) **não aplica** `NetworkPolicy` — os
policies existem no cluster mas são ignorados silenciosamente. Para que
`infra/security/network-policies.yaml` realmente restrinja tráfego, crie o
cluster com Calico no lugar do kindnet:

```bash
kind create cluster --name re-api-books --config kind-config.yaml
# desabilite o CNI padrão adicionando `networking.disableDefaultCNI: true`
# em kind-config.yaml, depois instale o Calico:
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.29.1/manifests/calico.yaml
```

Não obrigatório para acompanhar os outros manifests deste diretório — só
necessário se você quiser de fato validar que os `NetworkPolicy` bloqueiam
o que dizem bloquear.

## Sobre os 3 namespaces e Pod Security Admission

`namespace.yaml` cria `re-api-books-dev`, `-staging` e `-prod`, mas
`deploy.sh` só implanta workloads em `-dev` — os outros dois existem só
como estrutura para quando este projeto precisar deles. Vale registrar:
`-prod` está com `pod-security.kubernetes.io/enforce: restricted`, e
nenhum dos `Deployment`s deste diretório define `securityContext`
(`runAsNonRoot`, `allowPrivilegeEscalation: false`, etc.) — então, como
estão, esses manifests seriam **rejeitados** se aplicados em `-prod` hoje.
Isso é deliberado (não um bug escondido): adicionar `securityContext` a
todo container é um passo real antes de usar `-prod` de verdade, não algo
que faz sentido fazer só por fazer num namespace que ninguém usa ainda.

## Por que `imagePullPolicy: Never` em `movies-service`/`api-gateway`

As duas imagens são construídas localmente (`kind load docker-image`, sem
push para nenhum registry) — `Never` garante que o kubelet nunca tenta
puxar de um registry remoto uma imagem que só existe localmente. Trocar
para `IfNotPresent`/`Always` só faz sentido junto com um registry real
(ECR, Docker Hub, GHCR) — fora do escopo de um projeto de estudo sem custo.
