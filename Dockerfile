#  Build Stage ──
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Download dependencies first (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/kosku-backend ./cmd/api/main.go

#  Runtime Stage ─
FROM alpine:3.20

WORKDIR /app

# Runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/bin/kosku-backend .

EXPOSE 8080

ENTRYPOINT ["./kosku-backend"]
