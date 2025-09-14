# --- STAGE 1: Build ---
FROM golang:1.23-bookworm AS builder

WORKDIR /app


COPY go.work ./
COPY go.mod go.sum ./
COPY src/*/go.mod src/*/go.sum ./src/
COPY src/ ./src/
COPY cmd/ ./cmd/
COPY test/ ./test

RUN go work sync


RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /traffic_delays ./src/traffic_delays/

# RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /manager ./src/traffic_delays/manager

# --- STAGE 2: Run ---
FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl && \
    update-ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /traffic_delays .

RUN chmod +x /app/traffic_delays

EXPOSE 5004

ENTRYPOINT ["./traffic_delays"]
