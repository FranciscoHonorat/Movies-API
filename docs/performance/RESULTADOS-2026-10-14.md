# Bateria completa de testes — 2026-10-14

Relatório de **todos** os testes executados nesta rodada: race detector,
fuzzing, benchmarks, varredura de vulnerabilidades, carga, estresse, pico,
resistência (soak), pipeline de escrita assíncrona e caos. Todo número aqui
foi medido de fato; nada foi estimado. Os relatórios brutos de cada rodada
estão em [`raw/2026-10-14/`](raw/2026-10-14/).

## Sumário

| # | Tipo de teste | Ferramenta | Resultado |
|---|---|---|---|
| 1 | Unitários + integração com `-race` | `go test -race` | ✅ todos passam, **0 data races**. ❌ 1 fuzz seed falha (ver #2) |
| 2 | Fuzzing | `go test -fuzz` | ✅ 4 alvos, ~5,26 milhões de execuções sem falha. ❌ **`FuzzDeleteMovie` achou bug de overflow de `int32`** |
| 3 | Benchmarks | `go test -bench -benchmem` + `benchstat` | ✅ domínio em ns, handler HTTP em ~13,8µs |
| 4 | Vulnerabilidades | `govulncheck` | ⚠️ 5 CVEs alcançáveis em `golang.org/x/net@v0.58.0` (corrigido na v0.60.0) |
| 5 | Carga (100 e 500 req/s por endpoint) | vegeta | ✅ 100% em todos, **exceto paginação profunda** (❌ 0,13–6% de sucesso) |
| 6 | Estresse (rampa até quebrar) | vegeta | Listagem: 100% até **2.500 req/s**, quebra em 3.000. Mix: 100% até 1.000, quebra em 2.000 |
| 7 | Pico (spike) | vegeta | ⚠️ 10% de erro no pico; **sistema ainda falha ~4s depois** que a carga normaliza |
| 8 | Resistência (soak, 5 min) | vegeta + `docker stats` | ✅ 150.000 req, 100% sucesso, **sem vazamento de memória** |
| 9 | Escrita assíncrona (POST/DELETE) | vegeta + contagem direta no Mongo | ✅ 40.000 POST e 10.000 DELETE, 100% sucesso, sem perda |
| 10 | Caos: `movies-service` fora do ar | `docker stop/start` | ⚠️ recupera, mas com **~7–8s extras** após o serviço voltar |
| 11 | Caos: Mongo congelado | `docker pause` | ✅ falha rápida e recuperação em ~1s |
| 12 | Caos: latência de 250ms no Mongo | `tc netem` | ❌ **100% de falha** (só lento seria o esperado) |
| 13 | Caos: RabbitMQ fora do ar | `docker stop/start` | ❌ **consumer não reconecta** — mensagens ficam presas até reinício manual |
| 14 | Caos: SIGKILL no consumer | `docker kill -s KILL` | ❌ **2.719 de 20.000 mensagens perdidas (13,6%)** após 202 ao cliente |

## Ambiente

- **Data:** 2026-10-14
- **Máquina:** Intel Core i5-1135G7 (4 núcleos / 8 threads), Linux, Go 1.27.2.
- **Stack:** `docker compose up -d --build` local (Mongo, RabbitMQ, Jaeger,
  `movies-service`, `api-gateway`), incluindo as alterações não commitadas
  da working tree (`api-gateway/cmd/main.go`, `internal/middleware/`).
- **Dataset:** 28.451 filmes do seed, índices `_id_`, `title_1`, `year_1`.
- **Gerador de carga na mesma máquina** que o sistema testado: em taxas
  acima de ~2.000 req/s o vegeta compete por CPU com os containers, então
  os números de estresse são um limite inferior, não o teto real do sistema.
- **Higiene:** todos os 50.274 filmes e 60.273 jobs criados pelos testes
  foram apagados ao final; a base voltou a exatamente 28.451 filmes e 1 job
  (verificado com `countDocuments`). O contador de IDs (`NextID`) ficou
  avançado — inofensivo, IDs não são reaproveitados.

---

## 1. Testes unitários e de integração com `-race`

**Comando:** `go test -race -p 1 -count=1 -cover ./...` em cada módulo do
`go.work` (`movies-service`, `api-gateway`, `proto`, `shared`).

Os testes de integração exigem um Mongo **sem autenticação** em
`localhost:27017` (o do compose tem auth). Paramos só o Mongo do compose,
subimos um `mongo:7` descartável na 27017, rodamos e restauramos.

| Pacote | Resultado | Cobertura |
|---|---|---|
| `movies-service/internal/adapters/grpc-server` | ✅ ok | 95,2% |
| `movies-service/internal/adapters/mongodb` | ✅ ok (integração, Mongo real) | 57,0% |
| `movies-service/internal/adapters/seed` | ✅ ok (integração, Mongo real) | 78,9% |
| `movies-service/internal/core/domain/entity` | ✅ ok | 32,3% |
| `movies-service/internal/core/domain/valueobjects` | ✅ ok | 55,6% |
| `movies-service/internal/core/service` | ✅ ok | 92,6% |
| `movies-service/internal/adapters/rabbitmq` | sem testes | 0% |
| `api-gateway/internal/resilience` | ✅ ok | 84,4% |
| `api-gateway/internal/observability` | ✅ ok | 25,0% |
| `api-gateway/internal/handlers` | ❌ FAIL — só `FuzzDeleteMovie` (seeds de overflow, ver §2); os demais 5 testes passam com `-race` | 27,4% |
| `api-gateway/internal/adapters/rabbitmq`, `middleware` | sem testes | 0% |
| `proto`, `shared` | sem testes | — |

**Data races detectadas: 0.**

Extras: `go vet` limpo em todos os módulos. `gofmt -l` aponta
`api-gateway/cmd/main.go` (alteração não commitada da working tree).

Bruto: `raw/2026-10-14/race.txt`, `race-handlers-skip.txt`.

## 2. Fuzzing

O projeto **não tinha nenhum teste de fuzz**; foram criados nesta rodada:

- `movies-service/internal/core/domain/valueobjects/fuzz_test.go`
- `api-gateway/internal/handlers/movie_handler_fuzz_test.go`

Cada alvo verifica uma **propriedade** (não só "não dá panic"): ex.
`NewMovieTitle` aceita exatamente os títulos com `TrimSpace` não vazio e
≤ 300 bytes.

**Comando:** `go test -run=^$ -fuzz=^<Alvo>$ -fuzztime=45s`

| Alvo | Propriedade verificada | Execuções | Resultado |
|---|---|---|---|
| `FuzzNewMovieTitle` | aceita ⇔ trim não vazio e ≤ 300 bytes; não altera o título | 1.728.747 | ✅ PASS |
| `FuzzNewMovieYear` | aceita ⇔ `Atoi` ok e ano ∈ [1888, ano atual+2] | 1.730.320 | ✅ PASS |
| `FuzzNewMovieID` | aceita ⇔ id > 0 | 1.613.697 | ✅ PASS |
| `FuzzCreateMovie` | qualquer body → só 202 ou 400, sem panic | 187.641 | ✅ PASS |
| `FuzzDeleteMovie` | o ID enviado ao `movies-service` é o mesmo do path | — | ❌ **FAIL** |

### 🐛 Bug encontrado: overflow de `int32` em IDs da URL

```
id "4294967297" enviado ao movies-service como 1 (overflow de int32)
id "2147483648" enviado ao movies-service como -2147483648
```

`api-gateway/internal/handlers/movie-handler.go` faz `strconv.Atoi` (int de
64 bits) e depois `int32(idInt)` sem checar faixa. Consequência:
**`DELETE /api/v1/movies/4294967297` apaga o filme de ID 1.** O mesmo
padrão existe em `GetMovie` (linha 61) e em `page`/`limit` do `ListMovie`
(linhas 114–115, 131–132). Correção sugerida: `strconv.ParseInt(s, 10, 32)`,
que devolve erro para valores fora de `int32` → responder 400.

Bruto: `raw/2026-10-14/fuzz.txt`.

## 3. Benchmarks

Também **não existiam**; criados em:
`valueobjects/fuzz_test.go`, `entity/movie_bench_test.go`,
`handlers/movie_handler_fuzz_test.go`, `resilience/bench_test.go`.

**Comando:** `go test -run=^$ -bench=. -benchmem -count=5` (mediana das 5).

| Benchmark | Tempo/op | Memória/op | Alocações/op |
|---|---|---|---|
| `NewMovieID` | 1,06 ns | 0 B | 0 |
| `NewMovieTitle` | 23,5 ns | 16 B | 1 |
| `NewMovieYear` | 84,7 ns | 16 B | 1 |
| `NewMovieEntity` | 137,9 ns | 80 B | 3 |
| `NewMovieEntityForCreate` | 143,6 ns | 80 B | 3 |
| `Retry_SuccessFirstTry` | 6,2 ns | 0 B | 0 |
| `IsRetryableGRPC` | 40,9 ns | 0 B | 0 |
| `Client_GetMovie` (breaker + retry + timeout sobre fake) | 928,6 ns | 400 B | 6 |
| `CreateMovieHandler` (Gin + bind JSON + UUID, publisher no-op) | 13,8 µs | 7.979 B | 46 |

Leitura: o domínio é desprezível; o wrapper de resiliência custa < 1µs por
chamada; o custo do gateway está no handler HTTP (bind JSON + Gin), ~14µs —
ainda muito abaixo da latência de rede/Mongo medida nos testes de carga
(~1–2ms).

Bruto: `raw/2026-10-14/bench.txt`, `benchstat.txt`.

## 4. Vulnerabilidades (`govulncheck`)

Ambos os serviços chamam código vulnerável de `golang.org/x/net@v0.58.0`
(dependência indireta, via gRPC/OTel/Hermes):

| ID | Descrição |
|---|---|
| GO-2026-6617 | Crash de servidor HTTP/2 por race no encoder HPACK |
| GO-2026-6612 | Reembolso duplo de controle de fluxo em streams HTTP/2 |
| GO-2026-6611 | CPU excessiva por mudanças repetidas de janela inicial |
| GO-2026-6610 | Transporte HTTP/2 aceita headers de framing malformados |
| GO-2026-6603 | Exaustão de memória por headers Trailer em HTTP/2 |

**Correção:** `go get golang.org/x/net@v0.60.0` nos dois módulos.

Bruto: `raw/2026-10-14/govulncheck.txt`.

---

## 5. Teste de carga (vegeta)

Cada endpoint a 100 e 500 req/s por 30s. `cpu_peak` = maior `docker stats`
amostrado a cada 2s durante a rodada.

| Endpoint | Taxa | Sucesso | p50 | p95 | p99 | Máx | CPU pico (mongo / service / gateway) |
|---|---|---|---|---|---|---|---|
| `GET /health` | 100 | 100% | 0,48ms | 0,92ms | 1,62ms | 4,5ms | — / 0% / 4% |
| `GET /health` | 500 | 100% | 0,44ms | 0,96ms | 1,94ms | 7,2ms | — / 0% / 19% |
| `GET /movies?limit=10` | 100 | 100% | 1,91ms | 3,39ms | 4,97ms | 14,2ms | 42% / 14% / 14% |
| `GET /movies?limit=10` | 500 | 100% | 1,82ms | 4,78ms | 8,76ms | 59,5ms | 30% / 61% / 45% |
| `GET /movies/{id}` | 100 | 100% | 1,62ms | 2,97ms | 4,23ms | 14,9ms | 66% / 12% / 12% |
| `GET /movies/{id}` | 500 | 100% | 1,40ms | 2,94ms | 6,08ms | 39,0ms | 98% / 49% / 44% |
| `GET /movies?title=love` | 100 | 100% | 2,92ms | 5,85ms | 9,50ms | 39,8ms | 76% / 14% / 11% |
| `GET /movies?title=love` | 500 | 100% | 3,73ms | 15,90ms | 43,88ms | 121,2ms | 131% / 58% / 48% |
| `GET /movies/status/{id}` | 100 | 100% | 1,90ms | 3,37ms | 4,88ms | 9,9ms | 24% / 14% / 11% |
| `GET /movies/status/{id}` | 500 | 100% | 1,57ms | 3,30ms | 6,60ms | 44,9ms | 60% / 57% / 43% |
| **`GET /movies?page=500&limit=50`** | 100 | **6,33%** | 0,69ms | 533ms | 1.026ms | 1.150ms | **359%** / 21% / 10% |
| **`GET /movies?page=500&limit=50`** | 500 | **0,13%** | 0,50ms | 525ms | 550ms | 1.101ms | **436%** / 36% / 30% |
| Mix (400 URLs: ids aleatórios, páginas 1–100, busca) | 100 | 93,57%* | 1,72ms | 3,70ms | 5,15ms | 11,9ms | 61% / 12% / 11% |
| Mix | 500 | 100% | 1,49ms | 3,90ms | 7,38ms | 37,2ms | 93% / 42% / 38% |

\* Os 193 erros do mix a 100 req/s ocorreram **todos nos 2 primeiros
segundos**, logo após o teste de paginação profunda: o circuit breaker
ainda estava aberto. Ver "Breaker único" abaixo.

### ❌ Achado: paginação profunda derruba o endpoint

`page=500&limit=50` vira `skip(24950)`: isoladamente leva ~19ms, mas o
Mongo precisa percorrer 24.950 entradas do índice a cada request. Rampa
dedicada (20s por taxa):

| Taxa | Sucesso | p50 | p99 | CPU Mongo |
|---|---|---|---|---|
| 5–30 req/s | 100% | 17–23ms | 40–54ms | 17–86% |
| 50 req/s | 100% | 23ms | 69ms | 214% |
| **70 req/s** | **17%** | — | 1.063ms | **581%** |
| 85 req/s | 16% | — | 998ms | 464% |

O precipício fica entre 50 e 70 req/s. Quando a consulta passa do
`callTimeout` (500ms), o breaker abre e o gateway devolve **500** (não
503) para tudo. Correção natural: paginação por cursor
(`title > último_title`) em vez de `skip`, ou um teto para `page`.

### ⚠️ Achado: um breaker único para os 5 RPCs (falha em cascata)

`api-gateway/internal/resilience/client.go` usa **um** `gobreaker` para
`GetMovie`, `ListMovie`, `CreateMovie`, `DeleteMovie` e `GetMovieStatus`.
Uma consulta lenta (paginação profunda) abre o breaker e derruba também
`GET /movies/{id}`, que estava saudável. Além disso,
`handlers/errors.go` mapeia "circuit breaker is open" para **500**; o
código semanticamente correto é 503 (com `Retry-After`).

Bruto: `raw/2026-10-14/load*.txt`, `deeppage*.txt`.

## 6. Teste de estresse (rampa até quebrar)

15s por degrau, 8s de pausa entre degraus (para o breaker fechar).

| Cenário | Taxa | Vazão real | Sucesso | p50 | p99 | CPU (mongo / service / gateway) |
|---|---|---|---|---|---|---|
| Listagem | 1.000 | 1.000/s | 99,99% | 1,63ms | 25,6ms | 43% / 102% / 71% |
| Listagem | 2.000 | 2.000/s | 100% | 2,84ms | 75,5ms | 160% / 146% / 108% |
| Listagem | **2.500** | 2.499/s | **99,98%** | 1,65ms | 34,1ms | 179% / 170% / 117% |
| Listagem | 3.000 | 1.998/s | **66,7%** | 2,11ms | 485ms | 131% / 195% / 140% |
| Listagem | 4.000 | 2.633/s | 66,0% | 5,86ms | 495ms | 205% / 197% / 191% |
| Listagem | 6.000 | 506/s | 8,4% | 0,88ms | 858ms | 124% / 105% / 177% |
| Mix | **1.000** | 1.000/s | **100%** | 1,38ms | 11,4ms | 67% / 76% / 59% |
| Mix | 2.000 | 1.319/s | 66,1% | 11,9ms | 513ms | 249% / 131% / 99% |
| Mix | 3.000 | 627/s | 20,9% | 2,40ms | 601ms | 219% / 99% / 116% |

**Capacidade:** ~2.500 req/s na listagem simples e ~1.000–2.000 req/s no
tráfego misto (que inclui busca regex e páginas até 100). Acima disso o
modo de falha é sempre o mesmo: latência passa de 500ms → breaker abre →
vazão útil **despenca** (6.000 req/s entrega só 506/s com sucesso). Lembrete:
o vegeta divide os 8 threads da máquina com o sistema.

Bruto: `raw/2026-10-14/stress*.txt`.

## 7. Teste de pico (spike)

Listagem: 100 req/s (15s) → **2.500 req/s (10s)** → 100 req/s (20s), sem
pausa.

| Fase | Sucesso | p50 | p99 |
|---|---|---|---|
| Base (100/s) | 100% | 1,25ms | 5,4ms |
| Pico (2.500/s) | 89,7% | 11,0ms | 664ms |
| Depois (100/s) | **80,1%** | 1,22ms | 4,0ms |

Linha do tempo da fase "depois":
`0s: 0 ok/100 erro · 1s: 0/101 · 2s: 0/99 · 3s: 2/99 · 4s: 99/0 · …100% daí em diante`

⚠️ A carga já voltou ao normal, mas o sistema **continua rejeitando tudo
por ~4s** — tempo do breaker em estado aberto (`Timeout: 5s`). Os 2.500
req/s que passaram com 100% no estresse (§6) aqui deram 10% de erro porque
o pico chega sem aquecimento (pool de conexões frio).

Bruto: `raw/2026-10-14/spike*.txt`.

## 8. Teste de resistência (soak)

Mix a 500 req/s por **5 minutos** (150.000 requisições), `docker stats` a
cada 2s.

| | Valor |
|---|---|
| Sucesso | **100%** (150.000 × 200) |
| Latência | p50 1,66ms · p95 6,27ms · p99 12,12ms · máx 182,7ms |
| `api-gateway` memória | 64 MiB antes → oscila 58–83 MiB durante → 61 MiB depois |
| `movies-service` memória | 24 MiB antes → oscila 22–39 MiB durante → 31 MiB depois |
| `movies-mongo` memória | 174 MiB estável (picos pontuais de 266/335 MiB, voltam) |

✅ **Sem tendência de crescimento** — memória oscila (ciclos de GC) mas
não sobe ao longo dos 5 minutos. Sem indício de vazamento de memória nem
degradação de latência com o tempo.

Bruto: `raw/2026-10-14/soak-mix-500.*`.

## 9. Pipeline de escrita assíncrona

`POST /movies` só enfileira e devolve 202, então a vazão real de criação foi
medida contando documentos direto no Mongo a cada 0,5s (sem HTTP, para o
medidor não competir com o sistema — ver `docs/learning/0003`).

| Teste | Aceitação HTTP | Resultado real |
|---|---|---|
| POST 500 req/s × 20s | 10.000 × 202, p99 3,77ms | **10.000/10.000 criados**, acompanhando a taxa de publicação |
| POST 2.000 req/s × 15s | 30.000 × 202, p99 138ms | **30.000/30.000 criados** em ~21s (média ~1.430/s) |
| DELETE 500 req/s × 20s | 10.000 × 204, p99 5,18ms | 10.000 filmes descartáveis removidos |

Na rodada de 2.000 req/s, a criação ficou em ~1.200/s **enquanto** a
publicação acontecia e subiu para ~3.000/s quando a publicação parou —
publicação e consumo competem pelos mesmos recursos (CPU, Mongo).

Observação: `rabbitmqctl list_queues` mostrou **0 mensagens na fila o tempo
todo**, mesmo com backlog real. Motivo descoberto no §14: com
`autoAck=true` e sem `Qos`, o RabbitMQ empurra tudo imediatamente para o
buffer do processo — o backlog fica na memória do `movies-service`, não na
fila. Isso também explica o "`messages_ready` nunca saiu de 0" relatado no
`README.md` desta pasta (Fase 2).

Bruto: `raw/2026-10-14/write*.txt`, `ingest-count*.txt`.

---

## 10–14. Testes de caos

Em todos: carga constante durante a falha, resultado agregado em linha do
tempo por segundo (`*.timeline` no bruto). Script: `raw/2026-10-14/chaos.sh`.

### 10. `movies-service` fora do ar por ~20s

Mix a 200 req/s × 60s; `docker stop movies-service` em t=10s, `docker
start` em t≈32s.

| | |
|---|---|
| Sucesso total | 49,9% (5.990 ok / 6.010 erro) |
| Latência dos erros | p99 4,1ms — **falha rápida**, breaker funcionando |
| Desligamento | gracioso (log "sinal de encerramento recebido", `GracefulStop`) |
| Serviço de pé de novo | 16:58:30 |
| Gateway voltou a responder | 16:58:37 (**t=40s**) |

⚠️ ~7s de indisponibilidade **depois** do serviço já estar no ar. O probe
half-open das 16:58:32 falhou porque a conexão gRPC do gateway ainda
estava em backoff de reconexão; o breaker reabriu por mais 5s. Tempo de
recuperação ≈ timeout do breaker + backoff do gRPC (mesmo fenômeno de
`docs/learning/0005`).

### 11. Mongo congelado por 15s (`docker pause`)

| | |
|---|---|
| Sucesso total | 66,9% |
| Durante a pausa (t=10–25s) | 0% de sucesso, erros em ≤ 552ms (limitado pelo `callTimeout`) |
| Recuperação | **~1s** após `unpause` (t=26s: 72 ok; t=27s: 100%) |

✅ Comportamento ideal: falha rápida, sem acúmulo de requisições presas,
recuperação quase imediata.

### 12. +250ms de latência de rede no Mongo (`tc netem`)

Sidecar `nicolaka/netshoot:v0.13` na rede do container do Mongo, de t=10s
a t=30s.

| | |
|---|---|
| Sucesso total | 54,3% |
| Durante a latência | **~0–5 sucessos por segundo de 200** |
| Uma requisição isolada sob a mesma latência | ✅ 200 em **252ms** |
| Recuperação | ~3s após remover a latência |

❌ O esperado era "tudo 250ms mais lento", não "tudo falha". A requisição
sozinha cabe com folga no `callTimeout` de 500ms, mas sob 200 req/s falha.
**Hipótese (não verificada a fundo):** com 250ms por ida e volta, a
concorrência exige mais conexões no pool do Mongo; cada conexão nova faz
TCP + handshake + autenticação SCRAM (vários RTTs, > 1s), estourando os
500ms; o retry (3 tentativas) multiplica a carga e o breaker abre. Para
confirmar: medir com `SetMinPoolSize` alto ou métricas de pool do driver.
Na prática: uma degradação moderada do banco vira indisponibilidade total.

### 13. RabbitMQ fora do ar por 20s

POST a 100 req/s × 60s; `docker stop movies-rabbitmq` em t=10s, `start`
em t=30s.

| | |
|---|---|
| Durante a queda | 500 em todos os POSTs (breaker `rabbitmq-publish` aberto), falha rápida (p99 2,6ms) |
| Publicação após a volta | ✅ gateway reconectou sozinho em ~10s (t=40s) |
| POSTs aceitos (202) | 2.992 |
| Filmes criados | **1.012** |
| Mensagens na fila | **1.980 paradas, 0 consumers** |

❌ **O consumer do `movies-service` morre silenciosamente quando o RabbitMQ
cai e nunca reconecta** — nenhum log de erro. As mensagens não se perdem
(fila durável), mas ficam paradas até alguém reiniciar o serviço. Após
`docker restart movies-service`: fila drenou, 2.992/2.992 criados. Falta
tratar `NotifyClose` do canal/conexão em
`movies-service/internal/adapters/rabbitmq/consumer.go` (reconectar ou,
no mínimo, encerrar o processo para o orquestrador reiniciar).

### 14. SIGKILL no `movies-service` durante consumo de backlog

POST a 2.000 req/s × 10s (20.000 mensagens); `docker kill -s KILL
movies-service` em t=6s; `docker start` em t=9s.

| | |
|---|---|
| POSTs aceitos (202) | **20.000** |
| Filmes criados (contagem estável, 3 leituras) | **17.281** |
| **Perdidos** | **2.719 (13,6%)** |

❌ **Perda de dados.** `consumer.go` consome com `autoAck=true`: o RabbitMQ
considera a mensagem entregue no instante em que ela vai para o socket.
Sem `Qos`/prefetch, milhares de mensagens ficam no buffer do processo; um
crash descarta todas. O cliente recebeu 202 e um `correlation_id` para
mensagens que nunca serão processadas — não existe sequer o documento de
job, então o status fica "pending" para sempre. Correção: `autoAck=false`
+ `d.Ack()` só depois de gravar o filme e o job, com `ch.Qos(prefetch, …)`
para limitar o buffer.

---

## Achados priorizados

| Prioridade | Achado | Onde | Teste que revelou |
|---|---|---|---|
| 🔴 Alta | Perda de 13,6% das mensagens em crash (`autoAck=true`) | `movies-service/internal/adapters/rabbitmq/consumer.go` | Caos #14 |
| 🔴 Alta | `DELETE /movies/4294967297` apaga o filme 1 (overflow `int32`) | `api-gateway/internal/handlers/movie-handler.go` | Fuzz #2 |
| 🔴 Alta | Consumer não reconecta após queda do RabbitMQ | `consumer.go` | Caos #13 |
| 🟠 Média | Paginação profunda derruba o endpoint a partir de ~70 req/s | `movies-service/internal/adapters/mongodb/movie_repo.go` (`SetSkip`) | Carga #5 |
| 🟠 Média | Breaker único para todos os RPCs → falha em cascata | `api-gateway/internal/resilience/client.go` | Carga #5 |
| 🟠 Média | 250ms de latência no Mongo → 100% de falha | timeout 500ms + retry + pool | Caos #12 |
| 🟠 Média | 5 CVEs em `golang.org/x/net@v0.58.0` | `go.mod` dos dois serviços | govulncheck #4 |
| 🟡 Baixa | Breaker aberto responde 500 em vez de 503 | `api-gateway/internal/handlers/errors.go` | Carga/Caos |
| 🟡 Baixa | Recuperação lenta (~7s) após `movies-service` voltar | breaker + backoff do gRPC | Caos #10 |
| 🟡 Baixa | `gofmt` pendente em `api-gateway/cmd/main.go` | working tree | `gofmt -l` |

## Arquivos de teste criados nesta rodada

- `movies-service/internal/core/domain/valueobjects/fuzz_test.go` — 3 fuzz + 3 benchmarks
- `movies-service/internal/core/domain/entity/movie_bench_test.go` — 2 benchmarks
- `api-gateway/internal/handlers/movie_handler_fuzz_test.go` — 2 fuzz + 1 benchmark (`FuzzDeleteMovie` **falha de propósito** até o overflow ser corrigido)
- `api-gateway/internal/resilience/bench_test.go` — 3 benchmarks

## Como reproduzir

```bash
# 1. race (precisa de Mongo sem auth na 27017)
docker compose stop mongo && docker run -d --name test-mongo -p 27017:27017 mongo:7
for m in movies-service api-gateway proto shared; do (cd $m && go test -race -p 1 -count=1 -cover ./...); done
docker rm -f test-mongo && docker compose start mongo

# 2. fuzz (um alvo por vez)
(cd movies-service && go test -run=^$ -fuzz=^FuzzNewMovieTitle$ -fuzztime=45s ./internal/core/domain/valueobjects/)

# 3. benchmarks
(cd movies-service && go test -run=^$ -bench=. -benchmem -count=5 ./internal/core/...) > bench.txt
benchstat bench.txt

# 4. vulnerabilidades
(cd api-gateway && govulncheck ./...)

# 5–9. carga/estresse/pico/soak (a partir de raw/2026-10-14, que tem targets/)
./run.sh load-list-500 list 500 30s
./run.sh soak-mix-500 mix 500 300s

# 10–14. caos
./chaos.sh chaos2-mongo-pause mix 200 50s 10 "docker pause movies-mongo" 25 "docker unpause movies-mongo"
```
