# ADR 0004: Índices em `title` e `year` para corrigir o colapso da listagem sob carga

- **Status:** Implementado
- **Data:** 2026-09-07
- **Escopo:** performance (`movies-service`), descoberto seguindo o roteiro pessoal de testes de carga

## Contexto

Comecei a seguir meu roteiro de testes de carga contra o `docker-compose.yml`
deste repositório, rodando localmente por não ter ainda um deploy público
(ver "Próximos passos" em `docs/performance/README.md`). Ao medir
`GET /api/v1/movies?limit=10` (sem filtro, ordenação padrão por `title`)
com o dataset real de seed (28.451 filmes), encontrei um colapso abrupto:
estável até 60 req/s (p99 121ms), e entre 60 e 80 req/s a latência média
salta de 28ms para 1.22s (p99 de 121ms para 3.28s), com 0.2% de timeout em
100 req/s.

Confirmei a causa com `docker stats` durante o teste de 100 req/s:
`movies-mongo` em 582% de CPU, enquanto `api-gateway` e `movies-service`
ficavam abaixo de 10%. Investigando o motivo, encontrei que a coleção
`movies` só tinha o índice padrão de `_id` — nenhum índice em `title` ou
`year`, os dois campos usados por `ListMovies`
(`movies-service/internal/adapters/mongodb/movie_repo.go`) tanto para
filtro (`$regex` em `title`, igualdade em `year`) quanto para ordenação
(`sort_by`, exposto no handler REST como `?sort=title|year`). Sem índice
no campo de ordenação, o Mongo precisa escanear a coleção inteira e
ordenar em memória antes de aplicar `skip`/`limit` — em todo request,
independente de quantos resultados o cliente pediu.

## Decisão

Adicionei dois índices ascendentes simples, `title` e `year`, via uma
função nova `EnsureIndexes(ctx, collection)` em `movie_repo.go`, chamada
uma vez no boot de `movies-service` (`cmd/main.go`), logo depois do seed.
Usei `collection.Indexes().CreateMany` com uma definição fixa — isso é
idempotente (recriar um índice já existente com a mesma spec não faz
nada), então não me preocupei em guardar isso atrás de um "já rodei
antes?" explícito: rodar em todo boot é seguro e mantém os índices
presentes mesmo que alguém apague a coleção `counters`/`movies` e recrie
do zero.

Escolhi índices simples (B-tree ascendente), não um índice de texto
(`"text"`), porque o objetivo imediato era resolver a ordenação, que é
uma operação de igualdade/range normal — um índice de texto resolveria
melhor a busca por substring (`$regex` em `title`), mas mudaria a
semântica da busca (de substring livre para busca por token) e portanto o
contrato da API REST, o que não fazia sentido decidir sozinho no meio de
uma sessão de medição de performance.

## Consequências

**Positivas**
- Medido antes/depois, mesmo dataset, mesmas taxas de carga: em 100 req/s,
  a latência média caiu de 3.68s para 1.48ms (~2.500x), p99 de 6.86s para
  3.7ms (~1.850x). Em 500 req/s (5x a taxa que antes derrubava o sistema),
  100% de sucesso com p99 de 5.9ms.
- `docker stats` em 500 req/s pós-correção: `movies-mongo` em 24% de CPU
  (contra 582% em só 100 req/s antes) — o gargalo se moveu para os
  serviços Go (`movies-service` 42%, `api-gateway` 32%), o perfil esperado
  quando o banco deixa de ser o limitante.
- De brinde, o índice em `title` também acelerou
  `GET /movies?title=...` (busca com `$regex`), mesmo esse filtro não
  usando o índice diretamente — o Mongo consegue usar o índice para
  servir a ordenação sem precisar de um sort em memória separado.

**Negativas / pendências que assumi conscientemente**
- Não escrevi um teste Go automatizado para `EnsureIndexes` — validei
  manualmente via `mongosh` contra o `docker-compose` rodando de verdade
  (`db.movies.getIndexes()` antes e depois), o que me deu mais confiança
  sobre o efeito real medido do que um teste unitário teria, mas não
  protege contra uma regressão futura (alguém remover a chamada de
  `EnsureIndexes` do `main.go` sem que nenhum teste acuse isso). Fica como
  item pendente.
- `$regex` não-ancorado em `title` continua sem poder usar o índice para
  o filtro em si — só a ordenação se beneficia. Se a busca por texto virar
  um caso de uso importante, um índice de texto é o próximo passo, mas
  isso é uma mudança de contrato que quero decidir com calma, não como
  correção reativa de um teste de carga.
- Ainda não tenho um número "antes" para a busca por `title` — só medi
  esse endpoint depois de já ter aplicado o índice, então não incluí um
  comparativo antes/depois para ele em `docs/performance/README.md`
  (preferi não medir isso agora, sob risco de invalidar o antes/depois já
  medido do endpoint principal, a derrubar a stack de novo só pra ter o
  número "antes" da busca).

## Alternativas consideradas

- **Índice de texto (`"text"`) em `title` desde já:** rejeitei por ora —
  resolveria melhor a busca por substring, mas muda o significado de
  `?title=` na API (de "contém" para "contém uma palavra/token"), uma
  mudança de contrato que não cabe decidir no meio de uma sessão de
  medição sem confirmar antes.
- **Índice composto `{title: 1, year: 1}` em vez de dois índices
  separados:** um índice composto ajudaria só quando os dois campos são
  usados juntos numa mesma ordenação/filtro, o que `ListMovies` não faz
  (ordena por um campo de cada vez, via `sort_by`) — dois índices simples
  cobrem os dois casos de uso reais (`sort=title` e `sort=year`) sem essa
  limitação.
- **Rodar a criação de índice manualmente uma vez via `mongosh`, fora do
  código:** rejeitei — some da primeira vez que alguém recriar o ambiente
  (outro Mongo, outro cluster) sem lembrar do passo manual. Colocar em
  `EnsureIndexes`, chamado no boot, é reprodutível por construção.

## Validação

- Confirmei `go build ./...` e `go vet ./...` limpos em `movies-service`.
- Recriei a imagem Docker (`docker compose up -d --build movies-service`)
  e confirmei via `mongosh` (`db.movies.getIndexes()`) que os índices
  `title_1` e `year_1` existem, ao lado do `_id_` padrão.
- Medi antes e depois com `vegeta`, mesmas taxas de carga (20/40/60/80/100
  req/s, mais 300/500 só depois), contra o mesmo dataset de 28.451
  documentos — números completos em `docs/performance/README.md`.
- Confirmei a mudança de perfil de CPU com `docker stats --no-stream`
  durante ambas as rodadas (582% → 24% no `movies-mongo` em cargas
  comparáveis/maiores).

## Itens descobertos durante a implementação, fora do escopo desta ADR

- Falta teste automatizado para `EnsureIndexes` (ver "Negativas" acima).
- `$regex` não-ancorado em `title` não usa índice para o filtro — decisão
  de adicionar índice de texto fica para quando (e se) a busca por texto
  virar prioridade.
- Ainda não tenho deploy público — as métricas aqui são locais, o que o
  meu próprio roteiro de testes trata como pré-requisito pendente, não
  cumprido, para a Fase 0.
