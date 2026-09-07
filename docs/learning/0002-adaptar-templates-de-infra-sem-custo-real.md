# Lições: adaptar um template de infra genérico para um projeto sem custo real

- **Data:** 2026-09-07
- **Contexto:** adaptação de `infra/` para o re-api-books, ADR 0003

## 1. Um template de infra carrega suposições de stack que nem sempre se aplicam

O template original tinha `rds.tf` e `elasticache.tf` porque foi escrito
para um projeto com banco relacional e cache. re-api-books não tem
nenhum dos dois — copiar esses arquivos criaria Terraform que provisiona
recursos que nada no código usa. O sintoma de "copiei um template e não
sei para que serve metade dos arquivos" geralmente é esse: o template foi
generalizado a partir de UM projeto concreto, e generalização feita às
pressas carrega detalhes daquele projeto específico como se fossem
universais.

**Como aplicar:** antes de copiar um template de infra para um projeto
novo, mapear cada arquivo para uma necessidade real e concreta desse
projeto (`grep` no código por o que ele de fato usa — biblioteca de
banco, cliente de cache, etc.) é mais rápido do que copiar tudo e
descobrir na hora do `terraform apply` que metade dos recursos não tem
para quê existir.

**Referências:**
- The Twelve-Factor App, fator III ("Config") — <https://12factor.net/config> —
  o princípio geral por trás de "a configuração de infra deveria refletir
  o que o app de fato precisa, não um template genérico".

## 2. "Grátis" e "gerenciado" nem sempre andam juntos — verificar o modelo de cobrança, não supor

EKS cobra pelo control plane por hora rodando, independente de carga —
diferente de recursos "serverless" que só cobram por uso. Para um projeto
de estudo com a restrição explícita de custo zero, isso descarta EKS por
padrão, mesmo que o cluster fique a maior parte do tempo ocioso ou
completamente desligado entre sessões de estudo — a cobrança é por hora
de existência do control plane, não de uso.

**Como aplicar:** antes de adotar qualquer serviço gerenciado "só para
aprender o padrão", checar a página de pricing oficial e perguntar
especificamente "isso cobra por hora de existência, ou só por uso
efetivo?". `kind`/LocalStack não são "quase tão bons" como substituto de
aprendizado — para o propósito de aprender os padrões de Terraform/K8s,
ensinam exatamente a mesma coisa, sem o primeiro modelo de cobrança.

**Referências:**
- AWS EKS Pricing — <https://aws.amazon.com/eks/pricing/> — cobrança por
  hora de cluster, à parte do custo dos nós.
- LocalStack — <https://docs.localstack.cloud/user-guide/integrations/terraform/> —
  como apontar o provider Terraform da AWS para um emulador local via
  `endpoints {}`, sem mudar a sintaxe dos recursos.
- kind (Kubernetes IN Docker) — <https://kind.sigs.k8s.io/docs/user/quick-start/> —
  cluster Kubernetes real (mesma API, mesmos manifests) rodando inteiramente
  em containers Docker locais.

## 3. Um template pode conter peças que já não funcionam na versão atual da ferramenta

`security/pod-security-policies.yaml` do template original usa
`PodSecurityPolicy`, removido do Kubernetes na versão 1.25. Um manifesto
desse tipo, aplicado hoje, falha com `no matches for kind
"PodSecurityPolicy"` — não é uma questão de estilo ou preferência, é
código que não roda mais. Templates de infra, ao contrário de código de
aplicação com testes, não têm CI verificando se continuam válidos contra
a versão atual da plataforma — a obsolescência fica invisível até alguém
tentar aplicar.

**Como aplicar:** ao herdar um template de infra, verificar a versão de
cada ferramenta-alvo (aqui, Kubernetes) contra a data em que o template
foi escrito, e checar especificamente por remoções/depreciações conhecidas
antes de copiar às cegas.

**Referências:**
- Kubernetes, PodSecurityPolicy Deprecation — <https://kubernetes.io/docs/reference/access-authn-authz/psp-deprecation/> —
  remoção formal do PSP na 1.25 e o caminho de migração para Pod Security
  Admission.
- Kubernetes, Pod Security Admission — <https://kubernetes.io/docs/concepts/security/pod-security-admission/> —
  o substituto usado nesta adaptação (labels em `Namespace`, sem
  controller adicional).

## 4. Um sistema de CI/CD específico tem restrições estruturais, não só convenções

O template assumia que os arquivos de pipeline podiam morar em qualquer
lugar do repositório (`infra/cicd/.github/workflows/`). GitHub Actions
não permite isso — workflows só são descobertos em `.github/workflows/`
na raiz. Isso não é uma escolha de organização de projeto, é uma restrição
da própria plataforma. Uma adaptação de template que ignora isso produz
uma pasta que parece configuração real mas nunca executa — pior do que
não tê-la, porque engana uma leitura futura do repositório.

**Como aplicar:** ao adaptar um template de CI/CD, confirmar as restrições
estruturais da plataforma alvo (onde ela exige que arquivos morem, que
sintaxe aceita) antes de reproduzir a estrutura de diretórios do template
literalmente. A lógica reutilizável (scripts) pode morar em qualquer
lugar; a definição do pipeline em si, não.

**Referências:**
- GitHub Actions, Workflow syntax — <https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions> —
  workflows são descobertos exclusivamente em `.github/workflows/`.

## 5. Instrumentar antes de "escutar": um scrape config sem `/metrics` não ensina nada

Adicionar `infra/kubernetes/monitoring/prometheus.yaml` sem instrumentar
nenhum serviço teria produzido um Prometheus que faz scrape de um alvo
inexistente — tecnicamente "implementado", mas sem nenhum dado real para
aprender a ler um dashboard de verdade. Só depois de adicionar
`/metrics` no `api-gateway` (contador de requisições + histograma de
latência, rotulados pelo padrão de rota do Gin, não pela URL crua) o
Prometheus/Grafana deste projeto passam a ter algo real para mostrar.

O detalhe de rotular por `c.FullPath()` (`/movies/:id`) em vez da URL
literal (`/movies/42`) não é estético — é o que evita "cardinalidade não
limitada": cada valor distinto de label em uma métrica Prometheus cria
uma série de tempo própria, e uma métrica com uma série por ID de recurso
degrada a performance do Prometheus à medida que o número de recursos
cresce.

**Como aplicar:** ao adicionar observabilidade a um projeto, instrumentar
o código antes (ou junto) de configurar o coletor — um dashboard bonito
apontando para uma métrica que não existe é pior sinal de progresso do que
não ter dashboard nenhum. E sempre checar se um label de métrica é
limitado em cardinalidade (enum, rota, status) antes de o usar — nunca IDs
de usuário, IDs de recurso, ou timestamps como valor de label.

**Referências:**
- Prometheus, Metric and Label Naming — <https://prometheus.io/docs/practices/naming/>
- Prometheus, Instrumentation best practices (seção sobre cardinalidade) —
  <https://prometheus.io/docs/practices/instrumentation/#things-to-watch-out-for>
- Google SRE Book, capítulo 6 ("Monitoring Distributed Systems") —
  <https://sre.google/sre-book/monitoring-distributed-systems/> — os
  "quatro sinais de ouro" (latência, tráfego, erros, saturação) que o
  dashboard `infra/monitoring/grafana-dashboards/api-gateway.json` segue
  parcialmente (latência, tráfego, erros — sem saturação, já que este
  projeto não expõe métricas de recurso do processo ainda).
