# === Stage 1: Build binary ===
FROM golang:1.23-bookworm AS builder

WORKDIR /app


COPY go.work ./
COPY go.mod go.sum ./
COPY src/*/go.mod src/*/go.sum ./src/
COPY src/ ./src/
COPY cmd/ ./cmd/
COPY test/ ./test


RUN go work sync


WORKDIR /app/src/notification
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /notification .

# === Stage 2: Minimal runtime image ===
FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl && \
    update-ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /notification .

RUN chmod +x /app/notification

EXPOSE 6000

ENTRYPOINT ["./notification"]