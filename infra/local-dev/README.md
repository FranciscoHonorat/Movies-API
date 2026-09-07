# Local dev — re-api-books

O `docker-compose.yml` deste projeto continua na **raiz do repositório**,
não copiado/movido para cá. Duas razões concretas, não só preferência:

1. `.github/workflows/docker-image.yml` já roda `docker compose build` a
   partir da raiz — mover o arquivo exigiria atualizar essa referência (e
   qualquer hábito de `docker compose up` que já existia antes desta
   adaptação) só para bater com a estrutura do template original, sem
   ganho real.
2. Ter dois `docker-compose.yml` (um "real" na raiz, outro "de referência"
   aqui) é o mesmo problema de fonte-única-de-verdade já evitado em
   `infra/monitoring/README.md` e `infra/kubernetes/README.md` — path
   único, sempre.

Use o de sempre:

```bash
cp .env.example .env   # a partir da raiz
docker compose up --build
```

`init-scripts/` (do template original: `init-mongodb.sh`, `init-redis.sh`)
também não existe aqui: o Mongo é inicializado pela própria aplicação
(`movies-service` roda o seed de `movies.json` no boot — ver
`movies-service/internal/adapters/seed/`), e não há Redis neste projeto
(RabbitMQ é a única peça de mensageria/cache, e não precisa de
inicialização além do `docker-compose.yml` já existente).
