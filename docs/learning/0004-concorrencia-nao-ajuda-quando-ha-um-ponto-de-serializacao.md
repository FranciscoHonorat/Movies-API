# Lições: adicionar concorrência não ajuda quando existe um ponto de serialização

- **Data:** 2026-09-07
- **Contexto:** múltiplos consumers RabbitMQ, ADR 0005

## O que aconteceu

Pedi para mim mesmo (a pedido do usuário) provar que múltiplos consumers
no RabbitMQ fazem o throughput de criação de filmes crescer linearmente.
Implementei, medi, e o resultado real foi: 1 worker → 1.552/s, 2 →
1.708/s, 4 → 2.013/s, 8 → **1.726/s** (pior que 4). Sub-linear, com
regressão. `docker stats` durante o teste de 4 workers mostrou
`movies-mongo` em 77% de CPU e `movies-service` em 56% — nenhum dos dois
perto de saturar a máquina (a mesma `movies-mongo` já tinha sustentado
582% de CPU num teste anterior). CPU sobrando + throughput que não
escala é a assinatura de **contenção**, não de falta de paralelismo.

A explicação mais provável: cada mensagem processada faz um `$inc` no
**mesmo documento** MongoDB (o contador atômico de ID, decisão da ADR
0001) antes de qualquer outra coisa. Não importa quantas goroutines
processem mensagens em paralelo — todas elas, mais cedo ou mais tarde,
têm que esperar a vez de incrementar aquele único documento. É um ponto
de serialização por construção.

## Por que isso é um padrão conhecido, não uma peculiaridade deste projeto

Isso é uma instância direta da **Lei de Amdahl**: se uma fração do
trabalho de cada unidade de processamento é inerentemente serial (aqui,
o incremento do contador), o speedup possível ao paralelizar o resto é
limitado por essa fração serial, não importa quantos workers você
adicione — em algum ponto, adicionar mais workers só aumenta a fila de
espera pelo recurso serializado, sem aumentar o throughput real, e o
overhead de coordenar mais workers pode até piorar o resultado (o que
"8 pior que 4" sugere ter acontecido aqui). A lei foi formulada por Gene
Amdahl em 1967 para prever o limite de ganho de sistemas multiprocessador
— o raciocínio se aplica igual a um contador atômico num banco de dados
quanto a um processador com múltiplos núcleos.

## Como aplicar

- **Antes de adicionar concorrência para resolver um problema de
  throughput, identifique se existe um recurso compartilhado que toda
  unidade de trabalho precisa tocar.** Se existir, meça o throughput
  desse recurso isoladamente antes de prometer que mais workers vão
  ajudar — nesse projeto, isso seria medir a taxa de `$inc` sozinha,
  sem o resto do processamento, e comparar com o throughput medido do
  pipeline completo.
- **CPU ociosa sob carga é uma pista, não uma curiosidade.** Se o
  throughput não sobe mas nenhum recurso aparenta estar saturado
  (CPU, memória, I/O), a explicação normalmente é contenção de lock ou
  de recurso compartilhado, não falta de capacidade bruta — vale rodar
  `docker stats` (ou equivalente) durante qualquer teste de escala antes
  de concluir "precisa de mais hardware".
- **Um teste de carga que nunca acumula fila não prova nada sobre o
  consumidor.** Minha primeira tentativa de medir isso (via HTTP,
  `docs/performance/README.md` Fase 2) nunca chegou a formar um backlog
  real (`rabbitmqctl list_queues` mostrava `messages_ready: 0` o tempo
  todo) — sem backlog, não dá para saber se 1 ou 10 consumers dariam
  conta, porque o consumidor nunca chegou a ser testado no limite. Prove
  que o gargalo que você quer medir está de fato sendo exercitado antes
  de tirar conclusão da medição.
- **Reportar o resultado real, mesmo quando não é o que foi pedido, é
  mais valioso que forçar o número esperado.** "Provar escala linear"
  era o pedido; "descobrir e diagnosticar por que não escala linearmente"
  acabou sendo o resultado mais informativo — e é exatamente o tipo de
  raciocínio que se espera de alguém investigando performance de verdade,
  não só rodando benchmark até um número bonito aparecer.

## Referências

- Gene M. Amdahl, "Validity of the single processor approach to
  achieving large scale computing capabilities", AFIPS 1967 — o artigo
  original que formaliza o limite de speedup por paralelização quando
  existe uma fração de trabalho inerentemente serial.
