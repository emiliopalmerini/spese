# syntax=docker/dockerfile:1

########################
# Builder
FROM golang:1.25-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates gcc musl-dev && update-ca-certificates
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download || true
COPY . .
RUN CGO_ENABLED=1 go build -ldflags='-s -w' -o /out/spese ./cmd/spese

########################
# Runner
FROM alpine:3.22 AS runner
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/spese /app/spese

ENV SPESE_PORT=8081
EXPOSE 8081

ENTRYPOINT ["/app/spese"]
