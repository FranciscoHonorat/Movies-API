# ADR 0005: Múltiplos consumers no RabbitMQ — escala sub-linear, não linear

- **Status:** Implementado
- **Data:** 2026-09-07
- **Escopo:** `movies-service` (consumo de fila)

## Contexto

O `movies-service` processava `movies_queue` com uma única goroutine
(`for d := range msgs { handler(d) }` em
`internal/adapters/rabbitmq/consumer.go`). Eu queria adicionar consumo
paralelo e comprovar, com medição real, que o throughput cresce
linearmente com o número de workers — exatamente o que fiz, exceto pela
parte de crescer linearmente: **não cresce**. Registro isso aqui como
estava, não como eu esperava que estivesse.

### Por que a medição original (Fase 2, ADR anterior) não servia para testar isso

A ferramenta `ingest-load` publica via `POST /api/v1/movies` (HTTP). Eu já
tinha usado essa ferramenta para medir ~1.400 filmes/s "sustentados" (ver
`docs/performance/README.md`, Fase 2) e cheguei a interpretar isso como o
teto do consumidor único. Ao tentar usar a mesma ferramenta para testar
múltiplos workers, percebi um problema: rodando `rabbitmqctl list_queues`
durante a publicação (concorrência até 1.000), `messages_ready` **nunca
saiu de 0**. Ou seja, a fila nunca chegou a acumular — o consumidor único
já dava conta de consumir na mesma velocidade em que a mensagem chegava.
Isso significa que a medição anterior estava, na prática, limitada pelo
**gateway/HTTP** (a latência de cada `POST`, multiplicada pela
concorrência do cliente), não pelo consumidor — não dava para saber, só
com aquele método, se 2 ou mais workers ajudariam, porque nunca cheguei a
saturar 1 worker.

**Correção de método:** escrevi `tools/direct-publish/`, que publica
direto no RabbitMQ via AMQP, sem passar pelo `api-gateway` — 100.000
mensagens em ~1.5s (~65.000 msg/s), rápido o bastante para garantir um
acúmulo real na fila, isolando de vez a velocidade de consumo da
velocidade de produção. Combinei isso com uma amostragem de
`db.movies.countDocuments(...)` a cada 15ms (um script de `mongosh` de
processo único, para não pagar o custo de abrir um processo novo por
amostra) para medir a taxa de dreno da fila com boa resolução.

## O que a medição mostrou

| Workers | Taxa sustentada (janela de 35–45s, estado estável) |
|---|---|
| 1 | 1.552/s |
| 2 | 1.708/s (+10%) |
| 4 | 2.013/s (+30% sobre 1 worker) |
| 8 | 1.726/s (**pior que 4**, quase igual a 2) |

Não é crescimento linear (esperado com N workers: Nx o throughput de 1).
É crescimento **sub-linear que satura por volta de 4 workers e piora em
8** — um padrão clássico de contenção, não de paralelismo real.

**Por que, provavelmente:** cada mensagem processada faz 3 chamadas
sequenciais ao Mongo — `NextID` (`$inc` num único documento da coleção
`counters`), `CreateMovie` (insert) e `RecordJobCompleted`/`RecordJobFailed`
(`$set` num documento por `correlation_id`). Das três, `NextID` incrementa
**o mesmo documento** (`_id: "movie_id"`) em toda chamada, não importa
quantos workers existam — é um ponto de serialização por construção,
decisão já tomada na ADR 0001 (contador atômico sequencial em vez de
UUID/ObjectID, justamente para não quebrar o contrato de ID `int32`).
Suporta essa hipótese: com 4 workers processando ~2.000 msg/s, `docker
stats` mostrou `movies-mongo` em 77% de CPU e `movies-service` em 56% —
**nenhum dos dois perto de saturar a máquina** (a mesma `movies-mongo`
chegou a 582% de CPU no teste de índice da ADR 0004, então sabemos que há
CPU disponível de sobra). CPU ocioso + throughput que não escala é a
assinatura de contenção de lock/documento, não de limite de processamento.
Não instrumentei o tempo de cada uma das 3 chamadas individualmente para
provar isso com certeza total (ver "Itens descobertos").

## Decisão

1. **Implementei múltiplos workers mesmo assim.** `Consumer.Consume`
   (`movies-service/internal/adapters/rabbitmq/consumer.go`) agora recebe
   um parâmetro `workers int` e inicia essa quantidade de goroutines lendo
   do mesmo canal Go retornado por uma única chamada a
   `channel.Consume` — múltiplas goroutines consumindo o mesmo canal Go é
   um fan-out padrão e seguro (cada entrega vai para exatamente uma
   goroutine). Do lado do RabbitMQ isso continua sendo *um* consumer AMQP
   (`rabbitmqctl list_queues ... consumers` mostra `1`, não `N`) — o
   fan-out para N goroutines acontece só no lado da aplicação.
2. **Configurável via `CONSUMER_WORKERS`** (env var, `movies-service/cmd/main.go`),
   com **padrão 4** — o valor que empiricamente teve o melhor resultado
   entre os que testei (1/2/4/8). Adicionei a variável a
   `docker-compose.yml`, `.env.example` e `infra/kubernetes/configmap.yaml`.
3. **Mantive o ganho mesmo sendo sub-linear**: 4 workers processam ~30%
   mais rápido que 1, o que é uma melhoria real, só não do tamanho que eu
   esperava anunciar. Não fui atrás de "consertar" a contenção do contador
   agora (trocar para UUID, pré-alocar blocos de ID, etc.) — é uma mudança
   de contrato de ID maior, a mesma que a ADR 0001 já tinha descartado por
   esse motivo, e não vou revisitar essa decisão só por causa de um
   experimento de concorrência que não era o objetivo original.

## Consequências

**Positivas**
- ~30% de ganho de throughput real e mensurável no pipeline assíncrono
  (1.552/s → 2.013/s), sem custo de infraestrutura adicional.
- Descobri e documentei o gargalo de contenção do contador de ID — dado
  concreto para embasar uma futura decisão de mudar o esquema de geração
  de ID, se algum dia isso realmente importar (múltiplas réplicas de
  `movies-service`, por exemplo, cenário que a própria ADR 0001 já tinha
  cogitado).
- `tools/direct-publish/` fica disponível para testes futuros que
  precisem saturar a fila de propósito, sem depender do `api-gateway`.

**Negativas / pendências que assumi conscientemente**
- **Não entreguei o que foi pedido ao pé da letra** ("testa que a
  throughput cresce linearmente") — o resultado real é sub-linear com
  regressão acima de 4 workers. Decidi reportar isso como está, em vez de
  ajustar o teste até achar um cenário que "provasse" escala linear —
  seria desonesto com os próprios dados.
- Não instrumentei separadamente o tempo de `NextID` vs `CreateMovie` vs
  `RecordJobCompleted` para confirmar com certeza que o contador é
  especificamente o gargalo (é a explicação mais provável dado o desenho
  do código e a evidência de CPU ociosa, não uma certeza absoluta).
- `CONSUMER_WORKERS=8` tem desempenho pior que `4` nesta máquina — não
  investiguei a fundo por que (possíveis candidatos: overhead de
  scheduling de goroutines, limite do pool de conexões do driver do
  Mongo, contenção ainda maior no documento do contador). Fica registrado
  como não sabido, não como resolvido.

## Alternativas consideradas

- **Pré-alocar blocos de IDs por worker** (cada worker reserva, digamos,
  100 IDs de uma vez com um único `$inc` de 100, em vez de um `$inc` de 1
  por mensagem): reduziria a contenção no documento do contador por um
  fator de ~100, potencialmente destravando escala quase linear de
  verdade. Não implementei agora — é uma mudança de escopo maior que "só
  adicionar workers", e prefiro trazer isso como uma decisão à parte,
  não escondida dentro de uma ADR que já mudou de forma no meio do
  caminho.
- **UUID/ObjectID em vez de contador sequencial:** já descartado na ADR
  0001 pelo custo de mudar o contrato de ID em toda a API; esta ADR só
  reforça esse trade-off com dado real, não muda a decisão.
- **Não reportar a regressão em 8 workers e simplesmente recomendar "mais
  workers, sempre":** rejeitado — seria exatamente o tipo de número
  fabricado que o roteiro de testes deste projeto existe para evitar.

## Validação

- `go build ./...` e `go vet ./...` limpos em `movies-service`.
- Medido com `tools/direct-publish` (100.000 mensagens via AMQP direto,
  bypassando o `api-gateway`) + amostragem de `db.movies.countDocuments`
  a cada 15ms, para 1/2/4/8 workers — números na tabela acima.
- Confirmado, via `rabbitmqctl list_queues`, que a fila de fato acumula
  um backlog real durante o teste (ao contrário do método anterior via
  HTTP, onde `messages_ready` nunca saía de 0).
- Confirmado, via `docker stats`, que nem `movies-mongo` nem
  `movies-service` chegam perto de saturar CPU durante o teste de 4
  workers (77% e 56% respectivamente, contra os 582% que `movies-mongo`
  já demonstrou suportar na ADR 0004) — evidência a favor de contenção,
  não de limite de processamento.
- Dataset de seed confirmado intacto (28.451 documentos) após a limpeza
  dos dados de teste.

## Itens descobertos durante a implementação, fora do escopo desta ADR

- O contador atômico de ID (`counters` collection, `movies-service/internal/adapters/mongodb/movie_repo.go`)
  é um candidato forte a gargalo de escala se `movies-service` algum dia
  rodar com múltiplas réplicas gravando concorrentemente — o mesmo ponto
  que a ADR 0001 já tinha sinalizado como risco a reconsiderar, agora com
  evidência experimental (ainda que indireta) de que o padrão de
  contenção é real dentro de um único processo.
- Não sei por que `CONSUMER_WORKERS=8` regride em vez de só platô — vale
  instrumentação mais fina (latência por chamada individual ao Mongo) se
  alguém for investigar isso a fundo no futuro.
