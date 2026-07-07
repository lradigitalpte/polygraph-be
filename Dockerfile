# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /server ./server
COPY --from=builder /app/assets ./assets

ENV HOST=0.0.0.0
ENV PORT=8080
ENV GIN_MODE=release

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:${PORT}/health || exit 1

CMD ["./server"]
