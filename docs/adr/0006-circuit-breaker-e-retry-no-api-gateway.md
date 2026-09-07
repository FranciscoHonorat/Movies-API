# ADR 0006: Circuit breaker + retry no api-gateway — e dois bugs reais que encontrei testando isso de verdade

- **Status:** Implementado
- **Data:** 2026-09-07
- **Escopo:** `api-gateway` (chamadas de rede para `movies-service` e RabbitMQ)

## Contexto

O `api-gateway` faz duas chamadas de rede para dependências que não
controla: gRPC para `movies-service` (`GetMovie`, `ListMovie`,
`DeleteMovie`, `GetMovieStatus`, `CreateMovie`) e publish no RabbitMQ
(`CreateMovie`, via `output.MoviePublisher`). Nenhuma das duas tinha
qualquer proteção contra a dependência ficar lenta ou cair — uma
requisição HTTP simplesmente esperava o que desse e viesse.

Decidi implementar isso com dois padrões complementares: **retry com
backoff** para o tipo de falha que costuma se resolver sozinho em
milissegundos (uma reconexão, um pico passageiro), e **circuit breaker**
para parar de insistir numa dependência que já demonstrou estar fora do
ar, em vez de deixar cada requisição nova esperar de novo pelo mesmo
timeout. Ao testar isso contra o sistema real (não só com mocks), encontrei
dois problemas genuínos que mudaram a implementação — registro os dois
aqui porque são mais informativos que "implementei e funcionou de
primeira".

## Decisão

### 1. Biblioteca e desenho geral

Usei `sony/gobreaker/v2` (o `CircuitBreaker[T]` já é genérico nessa versão)
para o breaker, e escrevi um helper de retry pequeno (`resilience.Retry[T]`,
backoff exponencial com jitter completo, `api-gateway/internal/resilience/retry.go`)
em vez de trazer outra dependência para algo tão simples. Criei dois
decorators, ambos implementando a interface que já existia (`proto.MovieServiceClient`
e `output.MoviePublisher`), então **nenhum código de handler mudou** —
troquei só a montagem em `cmd/main.go`:

```go
client := resilience.NewMovieServiceClient(proto.NewMovieServiceClient(conn))
publisher := resilience.NewPublisher(newRabbitMQPublisher)
```

Cada chamada passa primeiro pelo retry, que por sua vez chama o breaker a
cada tentativa — assim, uma vez que o breaker abre, o retry para de
insistir imediatamente (não fica tentando de novo contra um breaker que já
disse "não" — ver "Erro que corrigi" nº 1 abaixo).

### 2. Classificação de erro — nem tudo deveria ser retentado, nem tudo deveria contar contra o breaker

Duas funções diferentes, propositalmente:

- `IsRetryableGRPC`: `Unavailable`, `DeadlineExceeded`, `ResourceExhausted`,
  `Aborted`, `Internal`, `Unknown` são retentados; `NotFound` e
  `InvalidArgument` não são — são respostas legítimas do serviço, retry
  não muda o resultado.
- `IsSuccessfulGRPC` (usada só para decidir se o breaker conta como falha,
  não se o chamador recebe erro): `NotFound`/`InvalidArgument` contam como
  **sucesso** do ponto de vista do breaker — o serviço respondeu
  corretamente, só que a resposta foi "não achei". Sem essa distinção, um
  cliente batendo em IDs inexistentes de propósito abriria o breaker para
  todo mundo, mesmo com `movies-service` saudável.

### 3. `CreateMovie` (gRPC) não tem retry automático

Ao contrário de `GetMovie`/`ListMovie`/`DeleteMovie`/`GetMovieStatus`,
`CreateMovie` via gRPC não é idempotente — `movies-service` gera um ID
novo a cada chamada. Retentar uma chamada cuja resposta se perdeu (mas que
já tinha sido processada do lado do servidor) criaria dois filmes. Dei a
esse método uma `RetryPolicy{MaxAttempts: 1}` separada. Na prática, hoje
isso é teórico: o handler HTTP de criação (`CreateMovie` em
`movie-handler.go`) não chama esse RPC — publica no RabbitMQ. Implementei
mesmo assim porque `Client` precisa satisfazer a interface inteira, e
"retry desligado de propósito" é uma decisão diferente de "esqueci de
configurar retry aqui".

### 4. Retry no publish do RabbitMQ é uma troca consciente, não isenta de risco

`RabbitMQPublisher.Publish` não usa confirmações de publicação (sem
`channel.Confirm`) — então um erro de rede no meio de um `Publish` pode
significar "a mensagem nunca chegou" ou "chegou, só a confirmação que se
perdeu". Retentar nesse segundo caso publicaria a mesma criação de filme
duas vezes, sob o mesmo `correlation_id` — o `movie_jobs` (indexado por
`correlation_id`) sobrescreveria o status de qual das duas completou por
último, mas as duas teriam sido de fato criadas, cada uma com seu próprio
ID, e uma delas ficaria "órfã" (sem nenhum job apontando pra ela). Decidi
aceitar esse risco por ora: sem confirmações habilitadas, não tenho como
distinguir os dois casos de qualquer forma, retry ou não — implementar
confirmações de publicação é a correção de verdade, e é uma mudança maior
que não cabia dentro desta ADR (ver "Itens descobertos").

## Erros que encontrei testando contra o sistema real, não só com mocks

Testes unitários (com um fake gRPC client e um fake publisher) passaram de
primeira. Só descobri os dois problemas abaixo derrubando de propósito os
containers reais (`docker compose stop movies-service` /
`docker compose stop rabbitmq`) e medindo o tempo de resposta do
`api-gateway` real.

### 1. Sem timeout por tentativa, uma falha demorava **20 segundos**

Minha primeira versão não aplicava nenhum timeout por tentativa — cada
chamada usava o contexto da própria requisição HTTP, sem prazo. Com
`movies-service` parado, a primeira requisição a `GET /movies/{id}`
**demorou 20.05 segundos** antes de sequer minha lógica de retry entrar em
ação: `grpc.NewClient` conecta de forma preguiçosa (lazy) e faz suas
próprias tentativas de reconexão internamente, com seu próprio backoff —
nada disso aparece como um erro que meu retry/breaker consegue ver até o
próprio gRPC desistir. Circuit breaker e retry são inúteis se uma única
tentativa pode travar por 20 segundos — a proteção real é limitar quanto
tempo uma tentativa individual pode levar.

**Corrigi** adicionando `callTimeout` (um `context.WithTimeout` por
tentativa, dentro de `executeBreaker`) a `Client` e `Publisher`. Testei
primeiro com 2s (baseado numa suposição conservadora) e medi de novo:
melhorou (6s para a 1ª falha em vez de 20s), mas ainda alto demais dado o
que eu já sabia sobre a latência real deste sistema — a Fase 1 dos meus
testes de carga (`docs/performance/README.md`) mediu p99 de poucos
milissegundos até em 500 req/s. Reduzi para **500ms**, generoso o
suficiente para uma chamada saudável, curto o suficiente para detectar uma
falha rápido:

| | Sem timeout | Timeout de 2s | Timeout de 500ms |
|---|---|---|---|
| Primeira falha detectada | 20.05s | ~6s (3 tentativas × ~2s) | ~1.5s (3 tentativas × ~500ms) |
| Depois do breaker abrir | (nunca chegava lá) | ~9ms | ~8-9ms |

### 2. O breaker do RabbitMQ nunca se recuperava — porque a conexão nunca se recuperava

Depois de derrubar e religar o `rabbitmq` container, esperei o `Timeout`
do breaker (5s) e tentei publicar de novo: continuava falhando, breaker
preso em `open` (visível em `/metrics`,
`circuit_breaker_state{breaker="rabbitmq-publish"} 2`, sem mudar mesmo
minutos depois). Não era o breaker que estava quebrado — era a conexão:
`RabbitMQPublisher` (`api-gateway/internal/adapters/rabbitmq/publisher.go`)
discava **uma vez**, na construção, e guardava esse canal para sempre.
Quando o RabbitMQ reiniciou, a conexão morreu de vez, e como o
`amqp091-go` não reconecta sozinho, todo `Publish` seguinte falhava para
sempre — não porque o RabbitMQ estava fora do ar (estava de volta), mas
porque o *cliente* nunca tentava se reconectar. Um circuit breaker
supõe que, passado o `Timeout`, uma nova tentativa vai contra uma conexão
que pelo menos *pode* funcionar — se o cliente nunca tenta de novo,
"esperar e tentar de novo" não ajuda em nada.

**Corrigi** reescrevendo `RabbitMQPublisher` para checar
`conn.IsClosed()`/`channel.IsClosed()` antes de cada publish e reconectar
sob demanda (`ensureConnected`, protegido por mutex) se necessário — ver
o arquivo para os detalhes. Confirmei a correção: parei o RabbitMQ,
publiquei (falha rápida, breaker abre), religuei o RabbitMQ, esperei o
`Timeout`, publiquei de novo — `202 Accepted` em 2.9ms, breaker de volta a
`closed`.

Isso não é um bug que criei ao adicionar o circuit breaker — o
`RabbitMQPublisher` original já tinha exatamente esse problema (todo
`Publish` depois de uma queda de conexão já falhava para sempre, com ou
sem breaker). O breaker só tornou o sintoma visível de um jeito novo
(estado preso em `open` no `/metrics`, em vez de um erro genérico
silencioso a cada request) — o suficiente para eu notar e corrigir agora.

## Observabilidade

Adicionei duas métricas Prometheus (`api-gateway/internal/resilience/metrics.go`,
mesmo `/metrics` que já existia — ver ADR 0003):
`circuit_breaker_state{breaker}` (0=closed, 1=half-open, 2=open) e
`circuit_breaker_trips_total{breaker}`. Atualizadas via o callback
`OnStateChange` do `gobreaker`, então refletem o estado real imediatamente,
não uma amostragem.

## Consequências

**Positivas**
- Falha de dependência agora tem latência limitada e previsível: no pior
  caso (breaker fechado, todas as tentativas falham), ~1.5s; com o breaker
  aberto, ~8-9ms — antes, uma dependência fora do ar podia significar
  dezenas de segundos por requisição.
- Estado do breaker visível em tempo real via `/metrics`, sem precisar
  inspecionar logs.
- Corrigi, de brinde, um bug real e pré-existente (reconexão do publisher
  RabbitMQ) que só apareceu porque testei contra containers de verdade
  caindo e voltando, não só contra mocks.

**Negativas / pendências que assumi conscientemente**
- Retry no publish do RabbitMQ pode, em teoria, criar um filme duplicado
  sob o mesmo `correlation_id` se uma falha de rede acontecer depois do
  broker já ter recebido a mensagem mas antes do cliente saber disso (ver
  "Decisão" nº 4). Não implementei confirmações de publicação para
  eliminar essa ambiguidade.
- Não escrevi teste automatizado para a reconexão do `RabbitMQPublisher`
  (precisaria de um RabbitMQ real subindo/derrubando durante o teste, não
  só um mock) — validei manualmente contra o `docker-compose` real, com
  os números acima, mas isso não me protege de uma regressão futura sem
  alguém rodar o mesmo teste manual de novo.
- `movies-service`, do lado do consumidor RabbitMQ, provavelmente tem o
  mesmo tipo de problema de reconexão (`consumer.go` também disca uma vez
  só) — não investiguei nem corrigi, porque estava fora do escopo desta
  ADR (é sobre `api-gateway`); registro como suspeita, não como fato
  confirmado.

## Alternativas consideradas

- **`cenkalti/backoff` para o retry, em vez de escrever na mão:** o
  padrão é simples o bastante (backoff exponencial + jitter, um loop) que
  prefiro não trazer outra dependência só pra isso — diferente do circuit
  breaker, cuja máquina de estados (closed/open/half-open, contadores,
  janelas de tempo) é sutil o bastante para valer a pena usar uma
  biblioteca madura em vez de reimplementar.
- **Circuit breaker também nas chamadas de `movies-service` para o
  MongoDB:** descartei — o driver do Mongo já tem seu próprio pool de
  conexões e (com `retryWrites`/`retryReads`, não configurado
  explicitamente aqui, mas o padrão do driver) sua própria lógica de
  retry para blips de rede; um circuit breaker sobre o próprio banco da
  aplicação, e não um serviço de terceiro, é uma decisão de escopo maior
  que eu não queria tomar de passagem dentro desta ADR.
- **Manter o timeout de 2s por tentativa:** descartei depois de medir —
  500ms já é ~50-100x a latência p99 real observada neste sistema (Fases
  1-4 dos testes de carga), então não é um valor arbitrário, é baseado em
  dado que eu já tinha.

## Validação

- `go build ./...`, `go vet ./...` e `go test ./...` limpos no
  `api-gateway`, incluindo os testes novos de `internal/resilience`
  (retry isolado, classificação de erro, e — o mais importante — que o
  breaker de fato para de chamar a dependência interna depois de abrir,
  com um fake gRPC client e um fake publisher controláveis).
- Medido contra o `docker-compose` real, não só unitário: parei
  `movies-service` e `rabbitmq` (separadamente), medi o tempo de resposta
  request a request até o breaker abrir, religuei cada um, esperei o
  `Timeout`, e confirmei recuperação (`200`/`202`, breaker de volta a
  `closed`, visível em `/metrics`) para os dois.
- Confirmado, antes da correção do timeout, o hang de 20.05s numa
  chamada real; confirmado, antes da correção do `RabbitMQPublisher`, que
  o breaker do RabbitMQ ficava preso em `open` mesmo minutos após o
  broker voltar.

## Itens descobertos durante a implementação, fora do escopo desta ADR

- Confirmações de publicação (`channel.Confirm` + esperar o ack) no
  `RabbitMQPublisher` eliminariam a ambiguidade do retry duplicar uma
  criação — não implementado agora.
- `movies-service/internal/adapters/rabbitmq/consumer.go` provavelmente
  tem o mesmo problema de não reconectar sozinho após queda de conexão —
  não confirmado nem corrigido nesta ADR.
- Nenhum circuit breaker ou retry foi adicionado nas chamadas de
  `movies-service` para o MongoDB — decisão consciente, não uma lacuna
  esquecida (ver "Alternativas consideradas").
