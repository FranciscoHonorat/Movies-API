# CI/CD — re-api-books

## Por que não existe `infra/cicd/.github/workflows/` de verdade

O template original coloca os workflows dentro de `infra/cicd/`. Isso não
funciona com GitHub Actions: o GitHub só descobre workflows em
`.github/workflows/` na raiz do repositório — não há como apontar para um
caminho alternativo. Os workflows reais deste projeto continuam onde já
estavam, em `.github/workflows/go.yml` (build+test dos 4 módulos Go, agora
com um serviço MongoDB — ver `docs/adr/0002`) e `.github/workflows/docker-image.yml`
(build das imagens Docker via `docker compose build`).

`infra/cicd/scripts/` guarda a lógica reutilizável por trás disso, para
rodar localmente sem depender do GitHub:

- `build.sh` — builda as duas imagens Docker.
- `test.sh` — roda `go build`/`go vet`/`go test -p 1` nos 4 módulos, na
  mesma ordem que o `go.yml` roda por matrix leg (requer um MongoDB em
  `localhost:27017`, igual ao `go.yml`).
- `deploy.sh` — builda, cria/reusa um cluster `kind` local, carrega as
  imagens nele (`kind load docker-image`, sem registry) e aplica todos os
  manifests de `infra/kubernetes/` e `infra/security/`.

Nenhum desses scripts é chamado pelos workflows do GitHub hoje — os
workflows têm sua própria lógica inline, principalmente porque o `go.yml`
usa uma matrix por módulo (relatando pass/fail de `api-gateway` e
`movies-service` separadamente na UI do Actions), o que um único script
"roda tudo" perderia. Ver `docs/learning/0002-*.md` para essa e outras
lições sobre adaptar um template de infra genérico para as restrições reais
de uma ferramenta específica.
