# === Stage 1: Build the Go binary ===
FROM golang:1.23-bookworm AS builder

WORKDIR /app


COPY go.work ./
COPY go.mod go.sum ./
COPY src/*/go.mod src/*/go.sum ./src/
COPY src/ ./src/
COPY cmd/ ./cmd/
COPY stationCodes.csv ./


RUN go work sync


WORKDIR /app/src/routeplanner
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /routeplanner .

# === Stage 2: Create minimal runtime image ===
FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl && \
    update-ca-certificates && \
    rm -rf /var/lib/apt/lists/*


COPY --from=builder /routeplanner .
COPY --from=builder /app/stationCodes.csv .

RUN chmod +x /app/routeplanner


EXPOSE 5002


ENTRYPOINT ["./routeplanner"]