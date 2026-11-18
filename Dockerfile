# Stage 1: Frontend React + TypeScript + Three.js
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend

COPY frontend/package*.json ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY frontend/ ./
RUN npm run build

# Stage 2: Backend Go + gopacket (libpcap) + SQLite
FROM golang:1.22-alpine AS backend-builder
WORKDIR /app/backend

RUN apk add --no-cache gcc musl-dev libpcap-dev sqlite-dev

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN go test -v -tags "libsqlite3" ./...
RUN CGO_ENABLED=1 go build -tags "libsqlite3" -ldflags="-s -w" -o /out/neuralnetwork ./cmd/server

# Stage 3: Runtime mínimo Alpine 3.20 con capacidades limitadas
FROM alpine:3.20 AS runtime
WORKDIR /app

RUN apk add --no-cache libpcap sqlite-libs libcap wget ca-certificates \
    && adduser -D -H -u 10001 appuser \
    && mkdir -p /app/public /data \
    && chown -R appuser:appuser /app /data

COPY --from=backend-builder /out/neuralnetwork /app/neuralnetwork
COPY --from=frontend-builder /app/frontend/dist /app/public

RUN cp /app/neuralnetwork /app/gateway \
    && setcap cap_net_raw,cap_net_admin=eip /app/neuralnetwork

USER appuser

ENV NN_INTERNAL_PORT=18080 \
    NN_PORT=8080 \
    NN_DB_PATH=/data/neuralnetwork.db \
    NN_STATIC_DIR=/app/public

EXPOSE 8080 18080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${NN_INTERNAL_PORT:-18080}/api/health" >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/neuralnetwork"]
