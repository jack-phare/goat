# Stage 1: Build the Go eval binary
FROM golang:1.24 AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /goat-eval ./cmd/eval/

# Stage 2: Python runtime with eval binary and dev tools
FROM python:3.12-slim

# Install dev tools needed by benchmarks
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    build-essential \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy the compiled Go binary
COPY --from=builder /goat-eval /opt/goat-eval
RUN chmod +x /opt/goat-eval

# Keep container running for sandbox exec
CMD ["tail", "-f", "/dev/null"]
