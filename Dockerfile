# syntax=docker/dockerfile:1

########################
# Builder
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download || true
COPY . .
# Ensure CA certificates are available to copy into the runner image
RUN apk add --no-cache ca-certificates && update-ca-certificates
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/spese ./cmd/spese
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/spese-worker ./cmd/spese-worker

########################
# Runner
FROM scratch AS runner
WORKDIR /app
COPY --from=builder /out/spese /app/spese
COPY --from=builder /out/spese-worker /app/bin/spese-worker
# Copy system CA bundle so HTTPS works inside scratch image
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Hint Go where to find the CA bundle in this image
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

ENV PORT=8081
EXPOSE 8081

ENTRYPOINT ["/app/spese"]
