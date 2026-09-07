# Lições: circuit breaker + retry protegem o padrão de chamadas, não a conexão em si

- **Data:** 2026-09-07
- **Contexto:** implementação de circuit breaker + retry no `api-gateway`, ADR 0006

## O que aconteceu

Implementei circuit breaker + retry para as duas chamadas de rede do
`api-gateway` (gRPC para `movies-service`, publish no RabbitMQ). Testes
unitários com mocks passaram de primeira. Só ao derrubar os containers de
verdade (`docker compose stop`) é que encontrei dois problemas que nenhum
teste com mock teria pego:

1. **Sem limitar o tempo de uma tentativa individual, uma falha
   demorava 20 segundos** — o cliente gRPC (`grpc.NewClient`, conexão
   preguiçosa) tenta se reconectar internamente, com seu próprio backoff,
   e nada disso vira um erro visível pro meu código até o próprio gRPC
   desistir. Circuit breaker e retry não adiantam nada se uma única
   tentativa pode travar por mais tempo do que o cliente que está esperando
   a resposta está disposto a esperar.
2. **O breaker do RabbitMQ nunca saía do estado `open`, mesmo minutos
   depois do broker voltar ao ar** — porque o cliente publisher discava
   uma vez só, na inicialização, e nunca reconectava sozinho. O breaker
   estava se comportando exatamente como devia (dando uma nova chance após
   o `Timeout`); só que essa nova chance ia contra uma conexão que já
   estava morta e continuaria morta para sempre, com ou sem breaker.

## Por que isso é um padrão conhecido, não uma peculiaridade deste projeto

Um circuit breaker resolve um problema específico: parar de repetir uma
chamada contra uma dependência que já demonstrou estar com problema, dando
tempo para ela se recuperar antes de tentar de novo (o padrão descrito por
Michael Nygard em *Release It!* — a origem do termo "circuit breaker" em
software, por analogia com o disjuntor elétrico que interrompe o circuito
antes que ele derreta). O que ele **não** resolve é qualquer estado
inválido que o *cliente* acumulou durante a falha — uma conexão TCP morta,
um handle de arquivo fechado, um cache local desatualizado. O breaker
assume implicitamente que, passado o tempo de espera, uma nova tentativa
tem uma chance real de funcionar; se o cliente nunca se recupera sozinho,
essa suposição é falsa, e o breaker fica "corretamente" preso em `open`
para sempre — o sintoma parece um bug no breaker, mas o breaker está
fazendo exatamente o que foi configurado para fazer.

Da mesma forma, retry com backoff (documentado, por exemplo, no Retry
pattern do Azure Architecture Center) resolve falhas transitórias — mas só
ajuda se cada tentativa individual falha rápido o suficiente para o
orçamento de tentativas fazer sentido. Um retry de "3 tentativas" não
significa nada se cada tentativa pode, sozinha, levar 20 segundos.

## Como aplicar

- **Todo circuit breaker/retry precisa de um timeout por tentativa
  configurado explicitamente, não herdado do contexto do chamador.** Não
  assuma que a chamada subjacente (um cliente gRPC, um driver de banco,
  uma biblioteca AMQP) vai falhar rápido sozinha — meça. Fiz isso medindo
  contra o sistema real (20s sem timeout, 6s com 2s de timeout, ~1.5s com
  500ms de timeout) em vez de supor um valor.
- **Antes de confiar na recuperação de um circuit breaker, pergunte: o
  que precisa ser verdade para uma nova tentativa ter sucesso?** Se a
  resposta inclui "o cliente precisa ter uma conexão viva com o servidor",
  confirme que o cliente de fato reconecta sozinho — bibliotecas de
  mensageria e alguns drivers de banco não fazem isso por padrão.
- **Teste contra o sistema real, não só com mocks, antes de declarar um
  padrão de resiliência pronto.** Os dois problemas acima só apareceram
  parando e religando containers de verdade e medindo o tempo de resposta
  — um teste unitário com um fake que "falha e depois volta a funcionar"
  não reproduz "o cliente nunca tenta de novo porque nunca reconecta", já
  que o fake, por construção, sempre está pronto para a próxima chamada.

## Referências

- Michael T. Nygard, *Release It!: Design and Deploy Production-Ready
  Software* (2ª edição, 2018) — o livro que introduziu e popularizou o
  padrão circuit breaker em software.
- Microsoft Azure Architecture Center, "Circuit Breaker pattern" —
  <https://learn.microsoft.com/azure/architecture/patterns/circuit-breaker>
- Microsoft Azure Architecture Center, "Retry pattern" —
  <https://learn.microsoft.com/azure/architecture/patterns/retry>
