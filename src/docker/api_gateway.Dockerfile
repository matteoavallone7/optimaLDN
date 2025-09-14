# --- STAGE 1: Build ---
FROM golang:1.23-bookworm AS builder

WORKDIR /app

# Copy workspace file and all modules
COPY go.work ./
COPY go.mod go.sum ./
COPY src/*/go.mod src/*/go.sum ./src/
COPY src/ ./src/
COPY cmd/ ./cmd/
COPY test/ ./test

# Sync dependencies in the workspace
RUN go work sync

# Build the user_service binary
WORKDIR /app/src/api-gateway
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /api-gateway .

# --- STAGE 2: Run ---
FROM debian:bookworm-slim

WORKDIR /app
COPY --from=builder /api-gateway .

RUN chmod +x /app/api-gateway

EXPOSE 8080

ENTRYPOINT ["./api-gateway"]
