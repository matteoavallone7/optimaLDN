# --- STAGE 1: Build ---
FROM golang:1.23-bookworm AS builder

WORKDIR /app

# Copy workspace file and all modules
COPY go.work ./
COPY go.mod go.sum ./
COPY src/*/go.mod src/*/go.sum ./src/
COPY src/ ./src/
COPY cmd/ ./cmd/

# Sync dependencies in the workspace
RUN go work sync

# Build the main service binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /traffic_delays ./src/traffic_delays/

# Optional: build Lambda manager separately if needed
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /manager ./src/traffic_delays/manager

# --- STAGE 2: Run ---
FROM debian:bookworm-slim

WORKDIR /app
COPY --from=builder /traffic_delays .
COPY --from=builder /manager .

RUN chmod +x /app/traffic_delays

EXPOSE 5004

ENTRYPOINT ["./traffic_delays"]
