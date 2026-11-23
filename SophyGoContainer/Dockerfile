# syntax=docker/dockerfile:1.7
#
# SophyGo для Yandex Serverless Containers (MAX-сборка).
#
# Контекст сборки — корень проекта SophyGoApp:
#   docker build --platform linux/amd64 \
#     -f /home/alex/Work/SophyGoApp/SophyGoContainer/Dockerfile \
#     -t cr.yandex/crp7hfn5k6uh54cfia4q/sophygo:v1 \
#     /home/alex/Work/SophyGoApp
#
# Образ собирается под linux/amd64 (среда выполнения Yandex Cloud),
# содержит ca-certificates для HTTPS-вызовов к Yandex Realtime API
# и слушает порт из переменной окружения PORT на 0.0.0.0.

# ---------- backend: Go-бинарник с apiKind=max ----------
FROM --platform=linux/amd64 golang:1.24-bookworm AS backend

WORKDIR /src/backend

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

# main.apiKind=max вшивает проверку initData MAX (вместо Telegram).
RUN go build \
        -trimpath \
        -ldflags="-s -w -X main.apiKind=max" \
        -o /out/sophygo-api \
        ./cmd/server

# ---------- frontend: production-сборка Vite ----------
FROM --platform=linux/amd64 node:22-bookworm-slim AS frontend

WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build

# ---------- runtime ----------
FROM --platform=linux/amd64 debian:bookworm-slim AS runtime

# ca-certificates обязательны: без них не работают HTTPS-вызовы к внешним API,
# включая Yandex Realtime (wss://ai.api.cloud.yandex.net).
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=backend /out/sophygo-api /app/sophygo-api
COPY --from=frontend /src/frontend/dist /app/frontend

# PORT задаёт сам сервис; 8080 — только значение по умолчанию для локального
# запуска. Приложение слушает 0.0.0.0:$PORT.
# Все записи — только во временный каталог /tmp (лимит 512 МБ).
ENV PORT=8080 \
    MESSENGER_API=max \
    STATIC_DIR=/app/frontend \
    EVENTS_FILE=/tmp/sophygo/events.jsonl \
    CLIENT_LOG_FILE=/tmp/sophygo/frontend-client.log \
    LOGGING_ENABLED=true \
    LOG_FORMAT=json \
    LOG_FILE=""

# EXPOSE — только документация, фактический порт берётся из PORT.
EXPOSE 8080

USER 10001:10001

ENTRYPOINT ["/app/sophygo-api"]
