# ============================================
# Stage 1: Build
# ============================================
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /openlist ./cmd/openlist/

# ============================================
# Stage 2: Runtime
# ============================================
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Create data directory
RUN mkdir -p /app/data

# Copy binary
COPY --from=builder /openlist /app/openlist

# Copy configs
COPY configs/ /app/configs/

VOLUME ["/app/data"]

EXPOSE 8080

ENTRYPOINT ["/app/openlist"]
CMD ["--scan"]
