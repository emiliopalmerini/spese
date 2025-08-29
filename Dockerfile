# syntax=docker/dockerfile:1

########################
# Builder
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/spese ./cmd/spese

########################
# Runner
FROM scratch AS runner
WORKDIR /app
COPY --from=builder /out/spese /app/spese

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/spese"]
