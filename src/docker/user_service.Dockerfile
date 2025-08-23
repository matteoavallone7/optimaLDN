# === Stage 1: Build ===
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
WORKDIR /app/src/user_service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /user_service .

# === Stage 2: Runtime ===
FROM  debian:bookworm-slim

WORKDIR /app

# Copy the compiled binary
COPY --from=builder /user_service .

RUN chmod +x /app/user_service

EXPOSE 5001
ENTRYPOINT ["./user_service"]
