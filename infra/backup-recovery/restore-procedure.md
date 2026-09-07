# Procedimento de restore — re-api-books

## Restore completo (desastre: banco perdido ou corrompido)

1. Confirme que o `mongo-deployment`/container está no ar e acessível (um
   restore precisa de um MongoDB vivo para restaurar dentro dele, mesmo que
   vazio).
2. Escolha o arquivo de backup mais recente e íntegro em `./backups/` (ou
   baixe do bucket S3/LocalStack: `aws s3 cp s3://<bucket>/<arquivo> .`).
3. Rode:
   ```bash
   mongorestore --uri="$MONGO_URI" --archive="movies-<timestamp>.archive.gz" --gzip --drop
   ```
   `--drop` remove as coleções existentes antes de restaurar — use sem
   `--drop` se quiser restaurar ao lado de dados atuais em vez de
   substituí-los (pode gerar conflito de `_id` com o contador de
   `movies-service`, ver ponto 4).
4. **Depois de um restore, reinicie os pods do `movies-service`.** O
   contador atômico de ID (`movies-service/internal/adapters/mongodb/movie_repo.go`,
   `NextID`) inicializa a partir do maior `_id` existente na coleção
   `movies` só na primeira chamada de cada processo (`sync.Once`). Se o
   restore trouxe de volta filmes com `_id` maiores do que o processo já
   tinha visto, um processo que não reiniciar pode gerar um ID que colide
   com um documento restaurado. Reiniciar força o processo a reler o `_id`
   máximo atual.
5. Valide: `GET /api/v1/movies?limit=1` deve devolver dados, e criar um
   filme novo (`POST /api/v1/movies` + `GET /movies/status/{id}`) deve
   funcionar sem erro de ID duplicado.

## Restore parcial (só quer inspecionar um backup antigo)

Restaure para um banco/coleção com outro nome, sem tocar no banco em uso:

```bash
mongorestore --uri="$MONGO_URI" --archive="movies-<timestamp>.archive.gz" --gzip \
  --nsFrom="movies.*" --nsTo="movies_inspect.*"
```

## O que este procedimento não cobre

Recuperação point-in-time (restaurar para um instante exato entre dois
backups) não é possível com a estratégia atual de dump periódico — ver
`backup-strategy.md`, seção "Por que não backup contínuo".
