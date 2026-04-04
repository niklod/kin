# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Download dependencies first (cached layer unless go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /relay \
      ./cmd/relay

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN adduser -D -u 1000 relay && mkdir -p /data && chown relay:relay /data

COPY --from=builder /relay /relay

# /data is where relay identity (Ed25519 key) is persisted across restarts
VOLUME ["/data"]

EXPOSE 7778/tcp

USER relay

ENTRYPOINT ["/relay"]
CMD ["--listen", "0.0.0.0:7778", "--config-dir", "/data"]
