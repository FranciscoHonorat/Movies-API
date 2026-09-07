# Lições: medir conclusão via polling pode medir a ferramenta, não o sistema

- **Data:** 2026-09-07
- **Contexto:** medição do pipeline assíncrono de criação, ADR 0004 / `docs/performance/README.md`

## O que aconteceu

Para medir o throughput real do pipeline assíncrono
(`POST /movies` → RabbitMQ → `movies-service` → conclusão), minha primeira
abordagem foi óbvia: publicar N filmes e, para cada um, consultar
`GET /movies/status/{correlationId}` repetidamente até ele completar.
Os números pareciam mostrar um sistema que degradava sob carga sustentada
— throughput caindo de 934/s (lote de 200) para 113/s (lote de 5.000), com
latência fim-a-fim subindo de 132ms para 19 segundos.

Isso parecia um achado real e preocupante. Não era. Trocando o método de
observação — de "consultar HTTP até completar" para "amostrar a contagem
de documentos direto no MongoDB, sem nenhuma requisição HTTP durante a
espera" — o mesmo pipeline, no mesmo lote de tamanho equivalente, sustentou
consistentemente **~1.400 criações/segundo**, mais de 2x o "teto" que a
medição anterior sugeria, sem nenhuma tendência de piora com o tamanho do
lote.

A causa: meu próprio polling de status virava tráfego real — para um lote
de 5.000 itens pendentes, isso significava milhares de requisições
HTTP→gRPC→Mongo por segundo só para *perguntar* "já terminou?", competindo
pelos mesmos recursos (conexões do Mongo, handler gRPC do
`movies-service`) que o processamento de verdade. Quanto maior o lote,
maior o volume de perguntas, e pior o resultado aparente — um efeito que
cresce com N, exatamente o padrão que eu tinha interpretado como "o
sistema degrada sob carga".

## Por que isso é um padrão conhecido, não uma peculiaridade deste projeto

Isso é uma instância do que a literatura de teste de carga chama de
**coordinated omission**: quando a ferramenta de medição só faz a próxima
observação depois que a anterior termina (ou, aqui, só considera um item
"resolvido" quando o próprio ato de perguntar recebe resposta positiva),
o tempo gasto perguntando passa a fazer parte do que está sendo medido,
distorcendo o resultado — sistematicamente pior quanto mais devagar (ou
mais carregado) o sistema sob teste estiver. Gil Tene (criador do
HdrHistogram e da ferramenta `wrk2`, feita especificamente para evitar
esse viés) tem uma palestra bastante citada sobre esse efeito ("How NOT to
Measure Latency"); a ferramenta `wrk2` — <https://github.com/giltene/wrk2>
— existe primariamente para corrigir esse problema em testes de carga HTTP
simples. O caso aqui é uma variação do mesmo problema: não era a
*ferramenta de disparo* de carga que sofria disso (o `vegeta` usado nas
Fases 1 não tem esse viés para requisições síncronas simples), mas a
*ferramenta de observação de conclusão assíncrona* que eu mesmo escrevi.

## Como aplicar

- Ao medir throughput de um pipeline assíncrono, prefira uma fonte de
  verdade que não passe pelo próprio sistema sob teste sempre que possível
  — aqui, contar documentos direto no banco em vez de perguntar pela API.
  Isso não é sempre viável (nem sempre você tem acesso direto ao estado
  interno), mas quando é, elimina a dúvida por completo em vez de só
  reduzir.
- Quando só resta polling via API, ajuste a frequência de consulta para
  não escalar com o número de itens pendentes — checar todos os N itens
  pendentes a cada tick faz o volume de perguntas crescer linearmente com
  N, exatamente o viés que eu queria evitar. Uma amostragem de tamanho
  fixo (poucos itens por tick, independente de quantos estão pendentes) é
  mais barata, ao custo de menos precisão por item individual.
- **Desconfie de "o sistema piora conforme a carga cresce" quando a prova
  vem de um teste que também aumenta proporcionalmente a sua própria carga
  de observação junto com N.** A pergunta certa antes de aceitar esse tipo
  de resultado: "o que exatamente eu fiz de diferente ao rodar com N maior,
  além de pedir para o sistema fazer mais trabalho?" Se a resposta inclui
  "também fiz mais perguntas para saber se já terminou", o resultado está
  contaminado até prova em contrário.

## Referências

- Gil Tene, "How NOT to Measure Latency" — a palestra (e o problema de
  coordinated omission que ela nomeia) por trás da ferramenta `wrk2`.
- `wrk2` — <https://github.com/giltene/wrk2> — ferramenta de carga HTTP
  desenhada especificamente para não sofrer desse viés, mantendo a taxa de
  disparo constante independente do tempo de resposta observado.
