# SophyGo в Yandex Serverless Containers (MAX-сборка)

Каталог `SophyGoContainer/` лежит в корне проекта и содержит Dockerfile,
скрипт публикации `deploy.py` и эту инструкцию.

Контейнер собирает backend SophyGo с `apiKind=max`, production-сборку frontend
и запускает один процесс, который сам отдаёт и API, и статику Mini App.

## Соответствие требованиям сервиса

| Требование | Как выполнено |
| --- | --- |
| Слушать `PORT` | `backend/internal/config/config.go`: `resolveAddr()` читает `PORT` и использует его приоритетно |
| Слушать `0.0.0.0` | Адрес собирается как `0.0.0.0:<PORT>` |
| `ca-certificates` | Устанавливаются в runtime-слое `debian:bookworm-slim` |
| `linux/amd64` | Все стадии `FROM --platform=linux/amd64`, сборка `--platform linux/amd64` |
| Запись только во временные файлы | `EVENTS_FILE` и `CLIENT_LOG_FILE` указывают в `/tmp/sophygo/`, лог backend идёт в stdout |
| `EXPOSE` — не источник истины | Порт берётся из `PORT`; `EXPOSE 8080` только для документации |

## Структура образа

- **backend** — `golang:1.24-bookworm`, сборка `-ldflags "-X main.apiKind=max"`;
- **frontend** — `node:22-bookworm-slim`, `npm ci && npm run build`;
- **runtime** — `debian:bookworm-slim` + `ca-certificates`, бинарник и `dist` фронтенда.

Backend отдаёт frontend сам (`STATIC_DIR=/app/frontend`), поэтому nginx внутри
контейнера не нужен: один процесс слушает `0.0.0.0:$PORT`.

## Сборка и публикация

Контекст сборки — корень проекта `SophyGoApp`, Dockerfile лежит в этом каталоге.

```bash
cd /home/alex/Work/SophyGoApp
python3 SophyGoContainer/deploy.py
```

Скрипт:

1. выполняет `yc container registry configure-docker` (авторизация через `yc`,
   без `docker login` и `key.json`);
2. собирает образ `cr.yandex/crp7hfn5k6uh54cfia4q/sophygo:v1`
   под `linux/amd64`;
3. публикует его в Yandex Container Registry.

Эквивалентные команды вручную:

```bash
yc container registry configure-docker

docker build --platform linux/amd64 \
  -f /home/alex/Work/SophyGoApp/SophyGoContainer/Dockerfile \
  -t cr.yandex/crp7hfn5k6uh54cfia4q/sophygo:v1 \
  /home/alex/Work/SophyGoApp

docker push cr.yandex/crp7hfn5k6uh54cfia4q/sophygo:v1
```

## Развёртывание ревизии

После появления образа в реестре ревизию контейнера `sophygo`
(`bba7hk3pi4suknk58vuu`) можно развернуть скриптом:

```bash
DEPLOY_REVISION=1 \
YC_SERVICE_ACCOUNT_ID=<сервисный-аккаунт> \
python3 SophyGoContainer/deploy.py
```

Или вручную:

```bash
yc serverless container revision deploy \
  --container-id bba7hk3pi4suknk58vuu \
  --image cr.yandex/crp7hfn5k6uh54cfia4q/sophygo:v1 \
  --platform linux/amd64 \
  --service-account-id <сервисный-аккаунт> \
  --cores 1 \
  --memory 512MB \
  --concurrency 8 \
  --execution-timeout 15s
```

## Переменные окружения ревизии

Обязательные секреты задаются в настройках контейнера, а не в образе:

```dotenv
MAX_BOT_TOKEN=<токен бота MAX>
YANDEX_API_KEY=<API-ключ Yandex Cloud>
YANDEX_FOLDER_ID=<folder id>
```

Необязательные:

```dotenv
CORS_ORIGINS=https://<адрес-контейнера>
LOGGING_ENABLED=true
LOG_FORMAT=json
```

`MESSENGER_API=max`, `STATIC_DIR`, `EVENTS_FILE`, `CLIENT_LOG_FILE` уже заданы
в Dockerfile. Если сервис передаёт свой `PORT`, он переопределит значение по
умолчанию.

## Локальная проверка образа

```bash
docker run --rm -p 8080:8080 \
  -e PORT=8080 \
  -e MAX_BOT_TOKEN=<токен> \
  -e YANDEX_API_KEY=<ключ> \
  -e YANDEX_FOLDER_ID=<folder> \
  cr.yandex/crp7hfn5k6uh54cfia4q/sophygo:v1

curl -sS http://127.0.0.1:8080/healthz
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/
```
