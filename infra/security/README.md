# Security — re-api-books

## O que tem aqui

- `network-policies.yaml` — default-deny + allow-list explícito entre
  `api-gateway`/`movies-service`/`mongo`/`rabbitmq`. Requer um CNI que
  aplique `NetworkPolicy` — ver `infra/kubernetes/README.md`
  ("Sobre `infra/security/network-policies.yaml` e o CNI do kind"), porque
  o CNI padrão do `kind` não aplica.
- Pod Security Admission — não é um arquivo separado, são labels em
  `infra/kubernetes/namespace.yaml` (`pod-security.kubernetes.io/enforce`).

## O que o template original tinha e não está aqui

- **`falco-rules.yaml`** (detecção de comportamento anômalo em runtime,
  Falco): omitido. Falco roda como DaemonSet privilegiado lendo eventos do
  kernel (eBPF/syscalls) — tecnicamente grátis (CNCF, open source), mas é
  uma peça de infraestrutura pesada e barulhenta para validar num cluster
  de 2 serviços de estudo, e exige tuning de regras para não afogar em
  falso-positivo. Vale reconsiderar se este projeto rodar em produção de
  verdade com tráfego real.
- **`pod-security-policies.yaml`** (PodSecurityPolicy): PSP foi **removido**
  do Kubernetes na 1.25 — um manifesto desse tipo simplesmente falharia
  (`no matches for kind "PodSecurityPolicy"`) em qualquer cluster atual.
  Substituído por Pod Security Admission (built-in, sem instalar nada) —
  ver `infra/kubernetes/namespace.yaml`.
- **`ssl-tls/`** (certificados/ACME): omitido porque não há domínio público
  neste projeto de estudo — `ingress.yaml` usa `re-api-books.local`
  (resolvido via `/etc/hosts`), e ACME (Let's Encrypt) só emite certificado
  para um domínio que resolve publicamente. Se este projeto algum dia for
  exposto num domínio real, `cert-manager` (grátis, OSS) + uma
  `ClusterIssuer` do Let's Encrypt é o caminho natural — não precisa do
  script `generate-certs.sh` do template original.
