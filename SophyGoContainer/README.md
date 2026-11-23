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
- **runtime** — `debian:bookworm-slim` + `ca-certificates` + `python3`.

Внутри образа раскладка как в репозитории:

```text
/app/backend/bin/sophygo-api   # бинарник MAX-сборки
/app/backend/.env              # секреты и настройки
/app/frontend/dist             # production-сборка frontend
/app/scripts/                  # start_services.py и общие скрипты
/app/deploy/                   # nginx/systemd/README
/app/SophyGoContainer/key.json # ключ сервисного аккаунта
```

Запуск идёт через `scripts/start_services.py`:

```text
ENTRYPOINT ["python3", "/app/scripts/start_services.py", "--only", "backend", "--foreground"]
```

`--foreground` заменяет процесс Python на backend-бинарник, поэтому PID 1 и
сигналы обрабатываются корректно. Backend сам читает `/app/backend/.env` и
отдаёт frontend из `STATIC_DIR=/app/frontend/dist`, nginx внутри не нужен.

### Секреты внутри образа

`.env` и `key.json` намеренно копируются в образ. Это удобно для локального
Docker/VM, но **небезопасно для публичного реестра**: любой, у кого есть право
`container-registry.images.puller`, может извлечь секреты из слоёв. Для
production надёжнее передавать их в ревизию через `--environment`/`--secret`.

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

## Переменные окружения

Секреты уже лежат в `/app/backend/.env` внутри образа. Значения из Dockerfile
имеют приоритет над `.env` (backend не перезаписывает уже заданные переменные
окружения), поэтому `MESSENGER_API=max`, `DEV_AUTH_USER_ID=0`, пути в `/tmp`
и `STATIC_DIR` зафиксированы. Если сервис передаёт свой `PORT`, он
переопределит значение по умолчанию.

При необходимости значения можно переопределить в ревизии:

```bash
yc --profile default serverless container revision deploy \
  --container-id bba7hk3pi4suknk58vuu \
  --image cr.yandex/crp7hfn5k6uh54cfia4q/sophygo:v1 \
  --environment MAX_BOT_TOKEN=<токен>,YANDEX_API_KEY=<ключ>,YANDEX_FOLDER_ID=<folder>
```

## Локальная проверка образа

Секреты уже внутри, поэтому достаточно порта:

```bash
docker run --rm -p 8080:8080 -e PORT=8080 \
  cr.yandex/crp7hfn5k6uh54cfia4q/sophygo:v1

curl -sS http://127.0.0.1:8080/healthz
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/
```

Запуск внутри контейнера вручную:

```bash
docker run --rm -it --entrypoint python3 \
  cr.yandex/crp7hfn5k6uh54cfia4q/sophygo:v1 \
  /app/scripts/start_services.py --only backend --foreground
```
