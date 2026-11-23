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
# Образ самодостаточный: внутри лежат .env, key.json, scripts/ и deploy/,
# а запуск идёт через scripts/start_services.py --foreground.
#
# ВНИМАНИЕ: .env и key.json попадают в слои образа. Любой, у кого есть
# доступ на pull из реестра, сможет их извлечь. Для production безопаснее
# передавать секреты в ревизию через --environment/--secret.

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
# python3 нужен для scripts/start_services.py.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates python3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Раскладка как в репозитории: start_services.py ищет backend/bin и frontend.
COPY --from=backend /out/sophygo-api /app/backend/bin/sophygo-api
COPY --from=frontend /src/frontend/dist /app/frontend/dist

# Конфиги и секреты (по требованию: образ самодостаточный).
COPY --chown=10001:10001 backend/.env /app/backend/.env
COPY backend/.env.example /app/backend/.env.example
COPY frontend/.env.example /app/frontend/.env.example
COPY --chown=10001:10001 SophyGoContainer/key.json /app/SophyGoContainer/key.json
COPY scripts/ /app/scripts/
COPY deploy/ /app/deploy/

# PORT задаёт сервис; 8080 — значение по умолчанию для локального запуска.
# Приложение слушает 0.0.0.0:$PORT.
# Все записи — только во временный каталог /tmp (лимит 512 МБ).
ENV PORT=8080 \
    MESSENGER_API=max \
    STATIC_DIR=/app/frontend/dist \
    EVENTS_FILE=/tmp/sophygo/events.jsonl \
    CLIENT_LOG_FILE=/tmp/sophygo/frontend-client.log \
    LOGGING_ENABLED=true \
    LOG_FORMAT=json \
    LOG_FILE="" \
    DEV_AUTH_USER_ID=0 \
    PYTHONDONTWRITEBYTECODE=1

# EXPOSE — только документация, фактический порт берётся из PORT.
EXPOSE 8080

USER 10001:10001

# start_services.py в foreground заменяет процесс Python на backend-бинарник.
ENTRYPOINT ["python3", "/app/scripts/start_services.py", "--only", "backend", "--foreground"]
