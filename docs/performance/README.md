# Performance — re-api-books

Métricas medidas seguindo o roteiro de testes de carga do Francisco
(Apache Bench/Vegeta + docker stats), rodadas localmente contra o
`docker-compose.yml` deste repositório (sem deploy público ainda — ver
"Ambiente" abaixo). Toda métrica aqui foi de fato executada; nada foi
estimado ou arredondado para parecer melhor. Os relatórios brutos de cada
rodada estão em `raw/`.

Este documento segue a ordem do roteiro original — Fases 1 a 4 são o
teste em si (abaixo), Fase 5 é o resumo compilado de tudo isso num só
lugar (**[pule direto para lá](#fase-5-métricas-compiladas)** se só quiser
os números finais).

## Ambiente

- **Data:** 2026-09-07
- **Onde:** `docker compose up -d --build`, localhost, sem tráfego externo
  concorrente às minhas medições.
- **Dataset:** 28.451 filmes (seed real de `movies-service/movies.json`,
  não dados sintéticos).
- **Ferramenta de carga:** [vegeta](https://github.com/tsenart/vegeta)
  (`go install github.com/tsenart/vegeta@latest` — não precisei de
  root/apt para instalar).
- **Ferramenta de recursos:** `docker stats --no-stream`.
- Isso é um teste **local**, não contra uma URL pública — ainda não
  satisfaz o critério de reprodutibilidade "qualquer um pode bater nessa
  URL e conferir" da Fase 0 do roteiro original. Documentado como
  pendência, não escondido: ver "Próximos passos".

## Fase 1: throughput & latência

### O achado: `GET /movies` sem índice colapsava acima de ~60 req/s

Antes de qualquer mudança de código, medi o endpoint de listagem
(`GET /api/v1/movies?limit=10`, sem filtro, ordenação padrão por
`title`) em várias taxas de carga:

| Taxa | Sucesso | Latência média | p99 |
|---|---|---|---|
| 20 req/s | 100% | 13.7ms | 31ms |
| 40 req/s | 100% | 20ms | 55ms |
| 60 req/s | 100% | 28ms | 121ms |
| **80 req/s** | 100% | **1.22s** | **3.28s** |
| **100 req/s** | 99.8%* | **3.68s** | **6.86s** |

\* 4 de 2000 requisições estouraram o timeout de 10s.

**Causa raiz:** a coleção `movies` (28.451 documentos) só tinha o índice
padrão de `_id`. `ListMovies` ordena por `title` por padrão — sem índice
em `title`, o Mongo faz um full collection scan + sort em memória em
**todo** request, mesmo pedindo só 10 resultados (`limit=10` não ajuda
quando o gargalo é ordenar a coleção inteira antes de aplicar o limite).
Confirmei isso com `docker stats` durante o teste de 100 req/s:

| Container | CPU |
|---|---|
| `movies-mongo` | **582%** |
| `api-gateway` | 7% |
| `movies-service` | 9% |

O gargalo era comprovadamente o banco, não os dois serviços Go — ambos
ficaram praticamente ociosos enquanto o Mongo saturava vários núcleos.

### A correção

Adicionei dois índices ascendentes, `title` e `year`
(`movies-service/internal/adapters/mongodb/movie_repo.go`, função
`EnsureIndexes`, chamada uma vez no boot do `movies-service`,
`cmd/main.go`) — os dois campos usados para ordenação e filtro em
`ListMovies`. `CreateMany` é idempotente: recriar um índice que já existe
com a mesma definição não faz nada, então é seguro rodar isso em todo
boot, não só uma vez manualmente.

### Depois: mesmo endpoint, mesmo dataset, mesmas taxas

| Taxa | Sucesso | Latência média | p99 |
|---|---|---|---|
| 20 req/s | 100% | 1.45ms | 4.1ms |
| 40 req/s | 100% | 1.27ms | 2.6ms |
| 60 req/s | 100% | 1.33ms | 2.9ms |
| 80 req/s | 100% | 1.45ms | 3.7ms |
| 100 req/s | 100% | 1.48ms | 3.7ms |
| 300 req/s | 100% | 1.29ms | 3.7ms |
| **500 req/s** | **100%** | **1.58ms** | **5.9ms** |

Em 500 req/s (5x a taxa que antes derrubava o sistema), `docker stats`
mostrou:

| Container | CPU |
|---|---|
| `movies-mongo` | 24% |
| `movies-service` | 42% |
| `api-gateway` | 32% |

O gargalo se moveu do banco para os serviços Go — o perfil esperado e
saudável (o trabalho residual é decodificar/serializar JSON e passar pela
pilha HTTP/gRPC, não mais varrer 28 mil documentos por request).

**Resumo da história antes/depois:** em 100 req/s, a latência média caiu
de **3.68s para 1.48ms** (~2.500x) e o p99 caiu de **6.86s para 3.7ms**
(~1.850x), sem nenhuma mudança de hardware — só os dois índices que
faltavam.

### Outros endpoints medidos

| Endpoint | Taxa | Sucesso | Latência média | p99 | Observação |
|---|---|---|---|---|---|
| `GET /health` | 100 req/s | 100% | 0.38ms | 1.1ms | Baseline — nenhuma consulta ao banco. |
| `GET /movies/{id}` (busca por `_id`) | 100 req/s | 100% | 1.0ms | 3.0ms | Já usava o índice padrão `_id`; não mudou com a correção — servia de controle para confirmar que o gargalo era específico da ordenação, não do Mongo em geral. |
| `GET /movies?title=love` (busca regex) | 100 req/s | 100%† | 2.4ms | 5.5ms | Só medido **depois** da correção — não tenho número "antes" para este (não rodei esse teste antes de aplicar o índice). O índice em `title` também acelera este caso porque o Mongo pode usá-lo para servir a ordenação sem sort em memória, mesmo com o filtro `$regex` não-ancorado não usando o índice para o próprio filtro. |

† Não reproduzi o cenário "antes" para a busca regex — não incluo esse
número na tabela de antes/depois por não tê-lo medido de fato (ver "Regra
de ouro" do roteiro original: "Se o teste não rodou, o número não entra").

### Como reproduzir (Fase 1)

```bash
docker compose up -d --build
go install github.com/tsenart/vegeta@latest   # ~/go/bin precisa estar no PATH

echo "GET http://localhost:8080/api/v1/movies?limit=10" > /tmp/target.txt
vegeta attack -targets=/tmp/target.txt -rate=100 -duration=15s | vegeta report

# CPU/memória durante a carga:
docker stats --no-stream
```

Relatórios brutos de cada rodada (formato texto do `vegeta report`,
`docker stats` capturado durante a carga): `docs/performance/raw/`.

## Fase 2 (adaptada): throughput do pipeline assíncrono de criação

O roteiro original mede ingestão via Redis Streams; este projeto não usa
Redis — a fila é RabbitMQ, e criação de filme é assíncrona
(`POST /movies` publica na fila; `movies-service` consome, cria o filme e
grava o resultado; o cliente confere via
`GET /movies/status/{correlationId}`). Um teste de carga HTTP comum não
mede isso: `POST /movies` só enfileira e devolve 202 na hora — não diz nada
sobre a velocidade real de criação.

Escrevi uma ferramenta pequena para isso,
[`tools/ingest-load/`](tools/ingest-load/) (Go puro, sem dependências),
que publica N filmes com concorrência configurável e mede o throughput real
de criação.

### A armadilha: medir "fim-a-fim" via polling HTTP mede a ferramenta, não o sistema

Minha primeira versão, depois de publicar, ficava consultando
`GET /movies/status/{correlationId}` de cada item até ele completar. Os
números pareciam mostrar um teto baixo e **piorando com o tamanho do
lote**:

| Lote (N) | Concorrência | Throughput sustentado medido | Latência média fim-a-fim |
|---|---|---|---|
| 200 | 20 | 934/s | 132ms |
| 1.000 | 50 | 664/s | 922ms |
| 2.000 | 50 | 411–337/s | 2.8–3.1s |
| 5.000 | 50 | 261–113/s | 9.3–19.1s |

Isso parecia um gargalo real do sistema degradando sob carga sustentada —
mas o volume de *consultas de status* que eu mesmo gerava crescia junto
com N (com 5.000 itens pendentes, meu polling chegava a disparar milhares
de requisições HTTP/gRPC por segundo só para *checar* status, competindo
pelos mesmos recursos — gRPC do `movies-service`, conexões do Mongo — que
o próprio processamento real). Troquei a estratégia de polling (por item,
contínuo → em lote, por tick) e os números mudaram, mas o padrão de
piorar com N se manteve — sinal de que o problema era estrutural na
abordagem de medição, não só um detalhe de implementação dela.

**Correção de método:** parei de usar HTTP para observar conclusão e passei
a amostrar `db.movies.countDocuments({title: "..."})` direto no MongoDB a
cada 0.5–1s (zero requisições HTTP durante a amostragem). Com N=20.000 e
concorrência de publicação 200:

```
t=1.94s  count=1.659
t=4.40s  count=5.784
t=8.05s  count=11.647
t=11.80s count=15.904
t=13.10s count=17.287
t=15.55s count=20.000  (completo)
```

A inclinação da parte linear da curva (t=1.94s a t=14.36s, evitando a
rampa inicial e a cauda final) dá **~1.400 criações/segundo** sustentadas
— mais de **2x** o "teto" que a medição via polling HTTP tinha sugerido, e
sem nenhuma tendência de piora com N. O suposto "gargalo que piora com
carga" era, na maior parte, a minha própria ferramenta de medição
competindo com o sistema medido.

### Números finais (método de contagem direta, sem polling HTTP)

| | Valor |
|---|---|
| Publish (aceitação HTTP, `POST /movies`) | 20.000 requisições, concorrência 200, 100% aceitas, latência média 22ms, p99 69ms |
| Criação real sustentada | **~1.400 filmes/segundo** (contagem direta no Mongo, N=20.000) |
| Tempo total para 20.000 filmes | 15.55s |

### Causa arquitetural (lida no código, não inferida do teste de carga) — e uma correção importante

`movies-service/internal/adapters/rabbitmq/consumer.go` processava
mensagens uma de cada vez, numa única goroutine
(`for d := range msgs { handler(d) }`). Na época eu interpretei o
throughput medido acima (~1.400/s) como o teto desse consumidor único.
**Essa interpretação estava incompleta**: rodando `rabbitmqctl
list_queues` durante aquele teste, `messages_ready` nunca saiu de 0 — a
fila nunca chegou a acumular, o que significa que eu nunca cheguei a
saturar o consumidor de fato; o número media, pelo menos em parte, a
velocidade de *publicação* via HTTP, não só a de consumo. A investigação
completa — incluindo o teste que efetivamente satura o consumidor e mede
sua taxa real — está em `docs/adr/0005-*.md` e resumida abaixo.

## Fase 2b: múltiplos consumers — escala sub-linear, não linear

Implementei consumo paralelo (`CONSUMER_WORKERS`, `movies-service/internal/adapters/rabbitmq/consumer.go`)
esperando provar escala linear. Não foi isso que a medição mostrou.

Para medir isso direito, publiquei direto no RabbitMQ via AMQP
(`tools/direct-publish/`, ~65.000 msg/s — rápido o bastante para garantir
um backlog real, confirmado via `rabbitmqctl list_queues`) e amostrei
`db.movies.countDocuments(...)` a cada 15ms:

| Workers | Taxa sustentada |
|---|---|
| 1 | 1.552/s |
| 2 | 1.708/s (+10%) |
| 4 | 2.013/s (+30%) |
| 8 | 1.726/s (**pior que 4**) |

Com 4 workers, `docker stats` mostrou `movies-mongo` a 77% de CPU e
`movies-service` a 56% — nenhum perto de saturar (a mesma `movies-mongo`
já sustentou 582% de CPU no teste da Fase 1). CPU sobrando + throughput
que não escala é a assinatura de **contenção**, não de falta de
paralelismo real: a explicação mais provável é o contador atômico de ID
(`movies-service/internal/adapters/mongodb/movie_repo.go`, `NextID`) —
um único documento Mongo incrementado por `$inc` a cada mensagem, ponto de
serialização por construção (decisão da ADR 0001), não importa quantos
workers concorrem por ele. Raciocínio completo, alternativas consideradas
e o porquê de eu não ter "corrigido" essa contenção agora:
`docs/adr/0005-*.md`. Lição generalizável sobre por que concorrência não
ajuda quando existe um ponto de serialização compartilhado:
`docs/learning/0004-*.md`.

Mantive o padrão em 4 workers (`CONSUMER_WORKERS=4`) — o melhor resultado
entre os testados, mesmo sem ser o crescimento linear que eu queria
mostrar.

## Fase 3: memória e CPU sob carga sustentada

Rodei `GET /movies` (28k docs, já com os índices da correção acima) a
300 req/s por 60 segundos (18.000 requisições, 100% sucesso, latência
média 812µs, p99 2.9ms), amostrando `docker stats --no-stream` a cada 5s.

| Container | Repouso | Sob carga (estável) | Depois da carga |
|---|---|---|---|
| `movies-service` | 12.2MB | 13.8–14.8MB | 12.9–13.4MB |
| `api-gateway` | 23.2MB | 29.6–34.9MB (sobe e estabiliza nos primeiros ~10s, não continua crescendo) | 33.4–33.7MB |
| `movies-mongo` | 478.5MB | ~450MB (estável) | ~450.6MB |
| `movies-rabbitmq` | 174.6MB | ~174MB (dois picos de CPU pontuais, sem relação com memória) | ~174.9MB |

**Sem sinal de memory leak**: nenhum container cresce continuamente durante
os 60s de carga sustentada, e todos voltam a CPU próxima de zero
imediatamente após a carga parar. A leve subida inicial do `api-gateway`
(23→34MB) é consistente com aquecimento normal de pool de conexões/GC, não
com vazamento — estabiliza e não volta a crescer pelo resto da janela.

## Fase 4: endpoints específicos que ainda faltavam

Os endpoints de leitura (`GET /movies`, `GET /movies/{id}`) e o pipeline de
criação já estavam cobertos nas Fases 1 e 2. Faltavam `GetMovieStatus` e
`DeleteMovie`:

| Endpoint | Carga | Sucesso | Latência média | p99 | Nota |
|---|---|---|---|---|---|
| `GET /movies/status/{id}` (job concluído) | 200 req/s, 20s | 100% | 860µs | 2.5ms | Mesma ordem de grandeza da busca por `_id` — faz sentido, é uma leitura indexada de dois documentos pequenos. |
| `DELETE /movies/{id}` | 100 req/s, 500 requisições reais | 100% | 923µs | 2.3ms | Testado contra 500 filmes descartáveis criados só para isso (`title: "Ingest Load Test"`), nunca contra o dataset de seed — ver "Higiene do teste" abaixo. |

**Higiene do teste:** todo filme criado para os testes de ingestão/delete
(`title: "Ingest Load Test"` / `"Status Test Movie"`) foi removido depois
de cada rodada — o dataset de seed voltou a exatamente 28.451 documentos
ao final de toda a sessão de testes, verificado via
`db.movies.countDocuments({})`.

## Fase 5: métricas compiladas

Resumo no formato do roteiro original (Throughput/Latência/Confiabilidade/
Recursos/Volume) — cada número aqui tem uma seção detalhada acima que
mostra exatamente como foi medido. Nenhum valor foi arredondado para
parecer melhor; onde a medição deu um resultado pouco impressionante (ex.:
memória praticamente não sobe sob carga), reportei isso mesmo assim.

### Throughput

- **Listagem (`GET /movies`, 28k docs, pós-correção de índice):** 500 req/s sustentados, 100% sucesso.
- **Criação assíncrona (`POST /movies` → RabbitMQ → `movies-service`):** 1.552/s com 1 consumer, **2.013/s com 4** (o padrão atual, `CONSUMER_WORKERS=4`) — escala sub-linear, não linear; ver Fase 2b e `docs/adr/0005-*.md` para o porquê.
- **Exclusão (`DELETE /movies/{id}`):** 100 req/s testados, 100% sucesso (não empurrei além disso — sem indício de que precisasse).
- **Consulta de status (`GET /movies/status/{id}`):** 200 req/s testados, 100% sucesso.

### Latência (endpoint mais crítico: listagem, pós-correção, 500 req/s — mesma rodada da medição de CPU abaixo)

- **P50:** 1.34ms
- **P95:** 3.13ms
- **P99:** 5.90ms
- **Max:** 23.96ms

(Antes da correção do índice, no mesmo endpoint a 100 req/s: P50 3.76s,
P99 6.86s — ver "O achado" acima para a tabela completa antes/depois.)

### Confiabilidade

- **Success rate (estado atual, todos os endpoints testados):** 100%.
- **Failed requests:** 0 (excluindo o cenário *antes* da correção do
  índice, deliberadamente incluído no antes/depois: 4 timeouts em 2.000
  requisições a 100 req/s = 99.8% de sucesso).
- **Error rate:** 0% no estado atual do código.

### Recursos

- **Memory (repouso):** `movies-service` 12.2MB · `api-gateway` 23.2MB · `movies-mongo` 478.5MB · `movies-rabbitmq` 174.6MB.
- **Memory (300 req/s sustentado, 60s):** `movies-service` ~14MB · `api-gateway` ~34MB · `movies-mongo` ~450MB · `movies-rabbitmq` ~174MB — **sem crescimento contínuo** (sem indício de leak; ver Fase 3).
- **CPU (500 req/s, listagem, pós-correção):** `movies-mongo` 24% · `movies-service` 42% · `api-gateway` 32%. Antes da correção, na taxa 5x menor de 100 req/s: `movies-mongo` **582%**.

### Teste de Volume

- **100.000 filmes publicados direto no RabbitMQ (bypassando o gateway):** consumidos a 1.552/s com 1 worker, 2.013/s com 4 (ver Fase 2b).

### Capturado em

- **Data:** 2026-09-07
- **Ferramentas:** [vegeta](https://github.com/tsenart/vegeta) (HTTP), ferramenta própria [`tools/ingest-load`](tools/ingest-load/) (pipeline assíncrono, Go puro), `docker stats` (recursos). Não usei Apache Bench — sem acesso a `sudo apt install` neste ambiente; vegeta cobre o mesmo caso de uso.
- **Ambiente:** **Local** (`docker compose up`), não produção — este projeto ainda não tem deploy público (ver "Próximos passos" abaixo). Sendo honesto sobre isso em vez de deixar implícito.
- **Comando representativo:** `vegeta attack -targets=targets-list.txt -rate=500 -duration=15s | vegeta report`

## Próximos passos

- **Deploy público real**, para satisfazer a Fase 0 do roteiro original
  (link que qualquer pessoa pode conferir, não só "rodei aqui na minha
  máquina"). Decidido usar MongoDB Atlas (M0) + CloudAMQP para
  banco/fila, ambos com tier gratuito permanente; falta decidir a
  plataforma de compute para `api-gateway`+`movies-service` (empacotados
  no mesmo container, já que só o gateway fala HTTP) — pausado por ora
  para priorizar as Fases 3/4 localmente.
- Múltiplos consumers já implementados (ADR 0005), mas a escala é
  sub-linear (satura por volta de 4 workers) por causa da contenção no
  contador atômico de ID. Se o throughput de criação precisar crescer de
  verdade no futuro, pré-alocar blocos de IDs por worker (em vez de um
  `$inc` de 1 por mensagem) é o próximo passo natural — não fiz isso agora
  por ser uma mudança de escopo maior que "adicionar workers".
- `GET /movies?title=...` com regex não-ancorado (`$regex` sem `^`) nunca
  vai usar um índice para o *filtro* em si, só para a ordenação — se a
  busca por título crescer em importância, um índice de texto
  (`db.movies.createIndex({title: "text"})` + `$text: {$search: ...}`) é
  o próximo degrau de performance, mas muda a semântica da busca
  (substring livre → busca por palavra/token) e portanto o contrato da
  API, então não fiz isso agora sem confirmar com você se é o
  comportamento que a busca deveria ter.
