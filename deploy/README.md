# Деплой SophyGo на VPS

Схема: **сборка полностью локальная**, на VPS загружаются только артефакты.
На VPS работают только два сервиса:

1. `sophygo-api.service` — Go-backend из готового бинарника;
2. `nginx` — раздаёт `frontend/dist` и проксирует API/WebSocket.

Go, Node, npm и Vite на VPS не нужны.

## Что где лежит

| Путь на VPS | Назначение |
| --- | --- |
| `/opt/sophygo/backend/bin/sophygo-api` | Готовый бинарник backend |
| `/opt/sophygo/backend/.env` | Production-переменные и секреты |
| `/opt/sophygo/frontend/dist` | Готовая production-сборка frontend |
| `/opt/sophygo/scripts` | Скрипты быстрого запуска (для локальной разработки) |
| `/opt/sophygo/deploy` | systemd-unit, конфиг nginx, скрипт деплоя, этот README |
| `/opt/sophygo/data` | `events.jsonl`, `backend.log`, `frontend-client.log` |

## Архитектура

- systemd-сервис `sophygo-api.service`:
  - `ExecStart=/opt/sophygo/backend/bin/sophygo-api`;
  - `EnvironmentFile=/opt/sophygo/backend/.env`;
  - слушает `127.0.0.1:8080`;
  - `Restart=always`, автозапуск при загрузке VPS.
- nginx:
  - `root /opt/sophygo/frontend/dist`;
  - `/api/` и `/healthz` → `127.0.0.1:8080`;
  - SPA-фолбэк на `index.html`;
  - TLS на edge делает Cloudflare, origin слушает 80/443.

Публичный адрес: `https://sophygo.pro`

## Обновление приложения

Из корня локального проекта:

```bash
SUDO_PASS='<пароль sudo>' python3 deploy/deploy.py
```

Скрипт:

1. собирает backend локально в `backend/bin/sophygo-api`;
2. собирает frontend локально в `frontend/dist`;
3. заливает только артефакты и конфиги;
4. останавливает сервисы, если они были запущены вручную через скрипты;
5. устанавливает unit-файл, включает и перезапускает `sophygo-api`;
6. обновляет конфиг nginx и перезагружает его;
7. проверяет `/healthz`, статику и состояние сервиса.

Пароль sudo передаётся на VPS через stdin и не попадает в аргументы
процессов и логи. Если `SUDO_PASS` не задан, sudo спросит пароль
интерактивно.

Production-файлы `backend/.env` и `frontend/.env.production` на VPS
не перезаписываются.

Полезные флаги:

```bash
# только загрузить артефакты и перезапустить, без сборки
python3 deploy/deploy.py --skip-build

# только перезапустить, без сборки и загрузки
python3 deploy/deploy.py --skip-build --skip-upload
```

## Управление на VPS

```bash
ssh SophyGo

sudo systemctl status sophygo-api
sudo systemctl restart sophygo-api
sudo journalctl -u sophygo-api -f

sudo nginx -t && sudo systemctl reload nginx
```

Логи backend также пишутся в `/opt/sophygo/data/backend.log`.

## Переменные backend

Production-значения в `/opt/sophygo/backend/.env`:

```dotenv
HTTP_ADDR=127.0.0.1:8080
EVENTS_FILE=/opt/sophygo/data/events.jsonl
CORS_ORIGINS=https://sophygo.pro
DEV_AUTH_USER_ID=0
LOGGING_ENABLED=true
LOG_FORMAT=json
LOG_FILE=/opt/sophygo/data/backend.log
CLIENT_LOG_FILE=/opt/sophygo/data/frontend-client.log

[telegram]
BOT_TOKEN=<токен Telegram-бота>
INIT_DATA_MAX_AGE=24h

[max]
BOT_TOKEN=<токен бота MAX>
INIT_DATA_MAX_AGE=1h
```

Ключи внутри секций получают префикс: `[telegram] BOT_TOKEN` читается
как `TELEGRAM_BOT_TOKEN`, `[max] BOT_TOKEN` — как `MAX_BOT_TOKEN`.
Поэтому один `.env` обслуживает оба мессенджера, а секреты
`YANDEX_API_KEY` и `YANDEX_FOLDER_ID` лежат там же и в репозиторий
не попадают.

## Сборка под Telegram или MAX

Мессенджер выбирается при сборке backend:

```bash
# Telegram (по умолчанию)
SUDO_PASS='<пароль sudo>' python3 deploy/deploy.py --api=telegram

# MAX
SUDO_PASS='<пароль sudo>' python3 deploy/deploy.py --api=max
```

Значение вшивается в бинарник через `-ldflags "-X main.apiKind=..."`.
Для локальной отладки его можно переопределить переменной окружения
`MESSENGER_API=telegram|max`.

Frontend один и тот же: он определяет мессенджер по непустому `initData`
(`window.Telegram.WebApp` или `window.WebApp` от MAX Bridge) и передаёт
его на backend в заголовке `X-Init-Data`.

## Размер маркеров

Масштаб маркеров достопримечательностей задаётся в
`/opt/sophygo/backend/.env`:

```dotenv
MARKER_SCALE=3
```

Frontend получает значение через `GET /api/v1/config` при загрузке,
поэтому пересобирать его не нужно. После изменения:

```bash
ssh SophyGo
sudo sed -i 's/^MARKER_SCALE=.*/MARKER_SCALE=4/' /opt/sophygo/backend/.env
sudo systemctl restart sophygo-api
```

Затем обновите страницу Mini App. `VITE_MARKER_SCALE` в frontend `.env` —
только fallback на случай, если backend недоступен.

## Запуск Mini App из Telegram

Кнопка меню бота уже настроена: `web_app` с текстом «Открыть SophyGo».
Telegram не умеет открывать Mini App автоматически при входе в чат —
запуск всегда требует действия пользователя. Чтобы запуск был в один
клик и работала прямая ссылка:

1. откройте `@BotFather` → `/mybots` → `SophyGoBot` → **Bot Settings** →
   **Configure Mini App** → **Enable Main Mini App**;
2. укажите URL `https://sophygo.pro/`;
3. проверьте результат: метод `getMe` должен вернуть
   `"has_main_web_app": true`.

После включения:

- в чате с ботом появляется кнопка «Открыть приложение»;
- ссылка `https://t.me/SophyGoBot/app` открывает SophyGo сразу,
  без промежуточного экрана бота.

## Локальная разработка

Скрипты `scripts/start_services.py` и `scripts/stop_services.py`
предназначены для локального запуска (Vite dev + `go run`). На VPS
они не используются в production-схеме.
