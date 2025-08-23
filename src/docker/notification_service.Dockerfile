# === Stage 1: Build binary ===
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

# Build the user_service binary
WORKDIR /app/src/notification
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /notification .

# === Stage 2: Minimal runtime image ===
FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /notification .

RUN chmod +x /app/notification
# Document internal port (optional)
EXPOSE 6000

ENTRYPOINT ["./notification"]