# Estratégia de backup — re-api-books

## O que precisa de backup

Só o MongoDB tem estado que não pode ser reconstruído. Tudo mais é
descartável e recriável a partir do código:

- **RabbitMQ**: mensagens em trânsito de um fluxo assíncrono de segundos
  (criação de filme). Perder a fila em um restart não perde dado de negócio
  — o pior caso é o cliente ter que repetir o `POST /movies`.
- **`movie_jobs` (coleção Mongo)**: estado efêmero de acompanhamento
  (pending/completed/failed por `correlation_id`). Não crítico de reter a
  longo prazo, mas está na mesma instância Mongo que `movies`, então o
  backup abaixo cobre os dois de graça.
- **`movies` (coleção Mongo)**: dado de negócio real — os filmes criados
  pelos usuários. Isso é o que importa preservar.

## Estratégia

`mongodump`/`mongorestore` (nativos do MongoDB, sem custo, sem serviço
gerenciado): `mongodb-backup.sh` faz um dump completo do banco `movies` e
grava um `.archive` compactado com timestamp. Rodando localmente (estudo,
sem nuvem), o destino é um diretório local (`./backups/` por padrão); se um
bucket S3/LocalStack estiver configurado (`BACKUP_S3_BUCKET`), o script
também sobe uma cópia para lá via `aws s3 cp` — ver
`infra/terraform/s3.tf` para o bucket usado em modo LocalStack.

Frequência sugerida para uso real (não implementada como cron neste
repositório de estudo — ver "Itens descobertos" na ADR): diária, com
retenção de 7 dumps diários + 4 semanais, via `CronJob` no Kubernetes
chamando este mesmo script dentro de um pod com `mongodump` instalado.

## Por que não backup contínuo / point-in-time recovery

MongoDB point-in-time recovery de verdade exige um replica set com oplog
(mínimo 3 nós) — deste projeto roda um único `mongo` standalone
(`mongo-deployment.yaml`, `replicas: 1`), então não há oplog para
capturar. Dump periódico é a estratégia correta para a topologia atual;
replica set + PITR é uma mudança de infraestrutura maior, fora do escopo
desta adaptação (ver ADR 0003, "Itens descobertos").
