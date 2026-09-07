# Lições: validação de binding e mapeamento de erros na fronteira dos adapters

- **Data:** 2026-09-07
- **Contexto:** auditoria e correções da ADR 0002 (re-api-books)

Este documento registra padrões generalizáveis descobertos durante a
auditoria — coisas que valem para o projeto como um todo, não só para os
bugs pontuais já corrigidos e registrados na ADR 0002.

## 1. Uma tag de struct mal formatada pode ser um bug de disponibilidade, não só de validação

`binding:"required, max=300"` (com espaço) parece um erro de estilo
inofensivo. Na prática, `go-playground/validator` trata cada tag separada
por vírgula como um nome de regra exato — `" max"` com espaço não é `"max"`
— e **entra em pânico** em vez de simplesmente ignorar a regra
desconhecida. Como o Gin roda com `Recovery()` por padrão, o pânico não
derruba o processo: ele vira um 500 silencioso em *toda* chamada aquele
endpoint recebe. O sintoma em produção (endpoint sempre 500) não aponta
óbvio para a causa (um espaço extra numa tag).

**Como aplicar:** qualquer tag `binding:`/`validate:` nova ou alterada
merece pelo menos um teste que efetivamente chame `ShouldBindJSON`/
`validator.Struct` com um payload real — não basta compilar (`go vet` não
pega isso) nem ler a struct visualmente. Vale considerar um teste genérico
que itera as structs com tag `binding` do pacote e garante que o bind não
entra em pânico com um payload mínimo válido, para pegar esse tipo de erro
de digitação estruturalmente, em vez de depender de lembrar disso a cada
struct nova.

## 2. Erros de infraestrutura crua não devem atravessar a fronteira do adapter

`movie_repo.go` devolvia `mongo.ErrNoDocuments` diretamente para quem
chamava `GetMovieByID`/`DeleteMovie`. As camadas de cima (`service`,
`grpc-server`) já tinham um mapeamento pronto (`toGRPCError`) para erros de
**domínio** (`errD.ErrMovieNotFound`), mas nunca recebiam esse erro — só o
tipo do Mongo, que caía no `default: codes.Internal`. O bug ficou invisível
por dois motivos combinados: (a) os testes de integração do repositório só
verificavam `assert.Error(t, err)`, nunca a identidade do erro; (b) os
testes do `grpc-server` usam mocks que já devolvem `errD.ErrMovieNotFound`
diretamente, então nunca exercitam o adapter Mongo de verdade.

Esse é o mesmo padrão que `movie_job_repo.go` já acerta:
`GetStatus` trata `mongo.ErrNoDocuments` explicitamente e devolve um
`JobStatus{Status: "pending"}` de domínio, nunca o erro cru do driver.
`movie_repo.go` deveria ter seguido o mesmo padrão desde o início.

**Como aplicar:** todo adapter que fala com um driver externo (Mongo,
RabbitMQ, um client gRPC/HTTP de terceiro) é responsável por traduzir os
erros sentinela desse driver (`mongo.ErrNoDocuments`, `sql.ErrNoRows`,
`grpc.ErrClientConnClosing`, etc.) para os erros de domínio do pacote
`err-d` **antes** de devolver. Nenhuma camada acima do adapter deveria
precisar conhecer um tipo de erro específico de infraestrutura. Ao escrever
o teste de integração de um adapter, prefira `errors.Is(err, errD.Algo)` a
`assert.Error(err)` puro — a segunda forma passa mesmo se o adapter vazar o
erro errado, que foi exatamente o que aconteceu aqui.

## 3. Um teste de integração que sempre falha em CI é pior do que nenhum teste

A ADR 0001 já tinha decidido, corretamente, que os testes de integração do
Mongo deveriam falhar rápido (não `t.Skip`) quando não há Mongo disponível,
para não mascarar infraestrutura quebrada — mas isso só funciona se **CI
prover a infraestrutura**. Sem o serviço Mongo no `go.yml`, esses testes
falhavam de forma 100% previsível em todo push, o que na prática treina
quem olha o CI a ignorar o vermelho ("ah, é só o Mongo, é sempre assim") —
o oposto do que a decisão da ADR 0001 queria. Um pipeline vermelho que
nunca fica verde não é sinal de nada; é ruído.

**Como aplicar:** ao adicionar um teste de integração contra uma
dependência real (banco, fila, cache), provisionar essa dependência em CI
faz parte do mesmo PR/tarefa — não é um item de "fase futura" separado. Se
não for possível provisionar imediatamente, o teste deveria nascer com
`t.Skip` condicional (com uma issue/ADR explicando por quê), não vermelho
por padrão.

## 4. Adapter novo sem teste é uma lacuna, mesmo com `go build`/`go vet` limpos

`movie_job_repo.go` foi implementado, plugado em `main.go`, exposto via
gRPC e HTTP, e mergeado — tudo funcionando de ponta a ponta segundo a
leitura do código — mas sem nenhum teste próprio. `go build`/`go vet`
limpos provam que o código compila e não tem os problemas óbvios que o
`vet` pega; não provam que o `$set`/upsert do Mongo faz o que o código
espera, ou que o caso "correlation_id nunca visto" realmente devolve
`"pending"` em vez de erro. O par `movie_repo.go` / `movie_repo_test.go` já
era o padrão estabelecido no próprio repositório — o adapter novo deveria
ter seguido o mesmo padrão simetricamente, no mesmo PR/commit.

**Como aplicar:** ao adicionar um adapter Mongo (ou similar) novo, replicar
a suíte de teste do adapter irmão mais próximo é o piso mínimo, não um
nice-to-have para depois.
