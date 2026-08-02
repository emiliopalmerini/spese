# syntax=docker/dockerfile:1

FROM node:24-alpine AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./frontend/
RUN --mount=type=cache,target=/root/.npm npm --prefix frontend ci
COPY frontend ./frontend
RUN npm --prefix frontend run build

FROM golang:1.25-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates gcc musl-dev && update-ca-certificates
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=frontend /src/web/dist ./web/dist
RUN CGO_ENABLED=1 go build -ldflags='-s -w' -o /out/spese ./cmd/spese
RUN CGO_ENABLED=1 go build -ldflags='-s -w' -o /out/spese-worker ./cmd/spese-worker
RUN CGO_ENABLED=1 go build -ldflags='-s -w' -o /out/spese-migrate ./cmd/spese-migrate

FROM alpine:3.22 AS runner
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata && addgroup -S spese && adduser -S -G spese spese && mkdir /data && chown spese:spese /data
COPY --from=builder --chown=spese:spese /out/spese /out/spese-worker /out/spese-migrate /app/

ENV SPESE_HOST=0.0.0.0 \
    SPESE_PORT=8080 \
    SPESE_DB_PATH=/data/spese.db \
    SPESE_TIMEZONE=Europe/Rome
EXPOSE 8080
USER spese

CMD ["/app/spese"]
