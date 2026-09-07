# Lições: mensageria assíncrona é o elo que a instrumentação automática não cobre

- **Data:** 2026-09-07
- **Contexto:** trace distribuído com OpenTelemetry + Jaeger, ADR 0007

## O que aconteceu

Instrumentar HTTP (`otelgin`) e gRPC (`otelgrpc`) neste projeto foi
trivial — duas linhas de configuração cada, e o contexto de trace passou
a atravessar as duas pontas automaticamente, sem eu precisar tocar em
nenhum handler. RabbitMQ foi diferente: não existe uma biblioteca de
instrumentação amplamente adotada para `amqp091-go`, e mesmo que
existisse, o problema de fundo continuaria — uma mensagem numa fila não
tem um request/response vivo para uma biblioteca instrumentar
magicamente. Tive que decidir, explicitamente, onde o contexto de trace
mora dentro da mensagem (os headers AMQP) e escrever os dois lados
(injetar ao publicar, extrair ao consumir) na mão.

Também descobri, tentando usar a instrumentação oficial de MongoDB
(`otelmongo`), que ela só suporta a v1 do driver — este projeto usa a v2
— e o `go get` só revelou isso na prática (baixou a v1 como dependência
indireta, um tipo incompatível com o `SetMonitor` da v2), não em
documentação que eu tivesse lido antes.

## Por que isso é um padrão conhecido, não uma peculiaridade deste projeto

HTTP e gRPC têm, por natureza, um objeto de requisição vivo durante toda
a chamada — colocar um header nele e ler do outro lado é natural, e é
exatamente isso que os padrões de propagação de contexto (o W3C Trace
Context, `traceparent`/`tracestate`, que é o que `propagation.TraceContext{}`
implementa) descrevem. Mensageria assíncrona quebra essa suposição: entre
publicar e consumir pode passar qualquer tempo, por qualquer caminho, e
não há "resposta" nenhuma — só a mensagem em si. A especificação de
convenções semânticas do OpenTelemetry para sistemas de mensageria
reconhece isso explicitamente, inclusive recomendando `span links` (não
uma relação direta de pai-filho) para o consumo, precisamente porque a
duração entre publicar e consumir pode ser arbitrariamente longa e não
deveria "contar" contra o span de quem publicou.

O caso do `otelmongo` é um lembrete mais simples, mas igualmente real:
"existe uma biblioteca de instrumentação oficial para X" não implica
"funciona com a versão de X que eu uso" — bibliotecas de instrumentação
seguem APIs específicas de versões específicas da biblioteca que
instrumentam, e migrações de major version (v1 → v2, aqui) são
exatamente o tipo de mudança que quebra esse contrato sem aviso na
documentação de alto nível.

## Como aplicar

- **Para qualquer transporte assíncrono (fila, tópico, webhook
  fire-and-forget), pergunte explicitamente: onde o contexto de trace vai
  morar dentro da mensagem, e quem garante que as duas pontas concordam
  sobre isso?** Aqui, a resposta foi "nos headers AMQP, via um tipo que
  as duas pontas compartilham" (`shared.AMQPHeaderCarrier`) — precisamente
  porque publicador e consumidor já tinham que concordar sobre o formato
  da mensagem em si (`MoviePublisherMessage`); o carrier de trace é mais
  uma peça do mesmo contrato, não algo à parte.
- **Antes de assumir que uma biblioteca de instrumentação "deve" suportar
  a versão que você usa, tente de fato (`go get`) e confira o que ela traz
  como dependência.** Aqui isso levou segundos e evitou escrever código
  contra uma API que nunca compilaria de verdade contra o driver certo.
- **Span "filho" é mais simples de implementar que span "link", mas nem
  sempre é a modelagem certa** — para uma fila com latência de consumo
  tipicamente baixa e previsível (este projeto: sub-segundo, medido nos
  testes de carga), a simplificação é aceitável e foi a escolha feita
  aqui; para uma fila onde mensagens podem ficar pendentes minutos ou
  horas, um "link" evita que a trace do publicador pareça ter demorado o
  tempo todo esperando o consumo.

## Referências

- W3C Trace Context — <https://www.w3.org/TR/trace-context/> — o formato
  (`traceparent`/`tracestate`) que `propagation.TraceContext{}` implementa
  e que este projeto usa para propagar contexto via HTTP, gRPC (metadata)
  e AMQP (headers, manualmente).
- OpenTelemetry Semantic Conventions for Messaging Systems —
  <https://opentelemetry.io/docs/specs/semconv/messaging/> — a
  especificação que discute span kind (producer/consumer) e a
  recomendação de usar links em vez de relações pai-filho diretas para
  consumo assíncrono.
