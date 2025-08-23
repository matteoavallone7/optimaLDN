# === Stage 1: Build the Go binary ===
FROM golang:1.23-bookworm AS builder

WORKDIR /app

# Copy workspace file and all modules
COPY go.work ./
COPY go.mod go.sum ./
COPY src/*/go.mod src/*/go.sum ./src/
COPY src/ ./src/
COPY cmd/ ./cmd/
COPY stationCodes.csv ./

# Sync dependencies in the workspace
RUN go work sync

# Build the user_service binary
WORKDIR /app/src/routeplanner
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /routeplanner .

# === Stage 2: Create minimal runtime image ===
FROM debian:bookworm-slim

WORKDIR /app

# Copy the compiled binary
COPY --from=builder /routeplanner .
COPY --from=builder /app/stationCodes.csv .

RUN chmod +x /app/routeplanner

# Expose the port your service uses (adjust if needed)
EXPOSE 5002

# Run the service
ENTRYPOINT ["./routeplanner"]