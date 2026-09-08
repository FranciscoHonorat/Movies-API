# syntax=docker/dockerfile:1
FROM golang:1.26.4-alpine AS builder

WORKDIR /app

# Hermes Go client isn't part of go.work (it's a separate checkout on the
# host, see docs/adr/0008-*.md) — copied to the exact absolute path its
# go.mod replace directives point at, so the same go.mod works unmodified
# for both host builds and this containerized one.
COPY --from=hermes-agent-go . /mnt/novovolume/arquivos/hermes-observability/packages/agent-go

COPY go.work go.work.sum ./
COPY movies-service/go.mod movies-service/go.sum ./movies-service/
COPY api-gateway/go.mod api-gateway/go.sum ./api-gateway/
COPY proto/go.mod proto/go.sum ./proto/
COPY shared/go.mod* shared/go.sum* ./shared/

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/server ./movies-service/cmd

FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/server .
COPY --from=builder /app/movies-service/movies.json ./movies.json

EXPOSE 50051

ENV MONGO_URI=mongodb://mongo:27017

CMD ["./server"]