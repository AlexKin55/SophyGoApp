# SophyGo Backend

Сервис на Go для Telegram Mini App SophyGo. Он:

1. проверяет `initData` от Telegram;
2. создаёт сессию голосового Гида;
3. проксирует WebSocket к Yandex AI Studio Realtime API, чтобы API-ключ
   не попадал в браузер;
4. принимает событие «пользователь открыл исторический объект» и отправляет
   в Realtime-сессию промпт `Расскажи об этом историческом объекте: <название>`;
5. сохраняет события в JSONL-файл;
6. поддерживает перебивание и команду «Замолчать».

## Схема работы

```text
Telegram Mini App
      │  POST /api/v1/session  (X-Telegram-Init-Data)
      ▼
Go backend ── проверка HMAC-подписи initData ── Telegram Bot Token
      │
      │  WebSocket /api/v1/realtime?token=<одноразовый токен>
      ▼
Go backend  ◄──────── WebSocket ────────►  Yandex Realtime API
   ▲                                         wss://ai.api.cloud.yandex.net
   │  PCM16 mono 16 кГц, base64 в JSON
   ▼
Telegram Mini App (микрофон + динамик)

Клик по объекту:
Mini App ── POST /api/v1/events/place-open ──► Go backend
                                             └─ conversation.item.create + response.create
                                                в активную Realtime-сессию
```

Yandex Realtime API сам выполняет STT, LLM и TTS в одной сессии. Server VAD
определяет начало речи пользователя, поэтому перебивание работает без
отдельной кнопки. Кнопка «Замолчать» отправляет `response.cancel` и очищает
очередь воспроизведения на клиенте.

## Требования

- Go 1.24 или новее.
- Токен Telegram-бота.
- API-ключ и folder id Yandex Cloud с доступом к Realtime API.

## Быстрый старт

```bash
cd backend
cp .env.example .env
# заполните TELEGRAM_BOT_TOKEN, YANDEX_API_KEY, YANDEX_FOLDER_ID
make run
```

Конфигурация сама читает `.env` из текущей папки или выше (уже заданные
переменные окружения имеют приоритет). `make run` дополнительно
экспортирует `.env` в окружение процесса. Значения с пробелами
записывайте в кавычках: `GUIDE_INSTRUCTIONS="... ..."`.

Сервис запускается на `http://localhost:8080`.

```bash
curl http://localhost:8080/healthz
# {"status":"ok","time":"..."}
```

Без `TELEGRAM_BOT_TOKEN` запросы с `initData` отклоняются. Без
`YANDEX_API_KEY`/`YANDEX_FOLDER_ID` голосовой Гид недоступен, но события
продолжают сохраняться.

## Отладка в браузере без Telegram

Telegram Mini App открывается внутри WebView и передаёт подписанный
`initData`. В обычном браузере его нет, поэтому для локальной отладки
предусмотрен dev-режим авторизации.

1. В `backend/.env` задайте пользователя, от имени которого будут
   выполняться запросы:

   ```env
   DEV_AUTH_USER_ID=1
   ```

   Если при этом заданы `YANDEX_API_KEY` и `YANDEX_FOLDER_ID`, голосовая
   сессия будет полностью рабочей. Без них `/api/v1/session` вернёт
   `realtimeAvailable: false`: карту, микрофон и VAD отладить можно, а
   ответы Гида — нет.

2. В `frontend/.env.local` разрешите подключение без `initData`:

   ```env
   VITE_DEV_AUTH=true
   ```

3. Запустите сервисы:

   ```bash
   cd backend && make run
   cd frontend && npm run dev
   ```

4. Откройте `http://localhost:5173`. Это secure context, поэтому
   `getUserMedia` и микрофон работают без HTTPS.

`DEV_AUTH_USER_ID` действует только когда заголовок
`X-Telegram-Init-Data` отсутствует: некорректная подпись по-прежнему
даёт `401`. В production переменную нужно оставить `0` (или удалить):
при старте сервис пишет предупреждение, если она включена.

Отладка WebSocket: Chrome DevTools → Network → WS →
`/api/v1/realtime`. В кадрах видны `session.update`,
`input_audio_buffer.append` (base64 PCM16) и
`response.output_audio.delta`.

## Логирование

По умолчанию логирование выключено. Включается мастер-флагом:

```env
LOGGING_ENABLED=true
```

- Backend пишет структурные логи в `LOG_FILE` и одновременно в stdout.
  Уровень задаётся `LOG_LEVEL` (`debug` даёт события WebSocket и Realtime),
  формат — `LOG_FORMAT` (`json` или `text`).
- Frontend в Telegram WebView не может писать файлы на устройстве. Поэтому
  браузер отправляет логи отдельными HTTP-пакетами на `POST /api/v1/client-logs`,
  а backend дописывает их в `CLIENT_LOG_FILE` на сервере. Пакеты ограничены
  `CLIENT_LOG_MAX_BODY_BYTES` и `CLIENT_LOG_RATE_PER_MINUTE`.
- По умолчанию всё складывается в `/tmp/sophygoapp/`:
  `backend.log`, `frontend.log` (вывод Vite dev-сервера) и
  `frontend-client.log` (логи браузера). Для production лучше указать
  `/var/log/sophygoapp/` и настроить logrotate: `/tmp` может очищаться.

## Переменные окружения

| Переменная                 | По умолчанию                                              | Назначение                                             |
| -------------------------- | --------------------------------------------------------- | ------------------------------------------------------ |
| `LOGGING_ENABLED`          | `false`                                                   | Мастер-флаг логирования                                |
| `LOG_FILE`                 | `/tmp/sophygoapp/backend.log`                             | Файл логов backend                                     |
| `LOG_LEVEL`                | `info`                                                    | Уровень: debug, info, warn, error                      |
| `LOG_FORMAT`               | `json`                                                    | Формат: json или text                                  |
| `CLIENT_LOG_FILE`          | `/tmp/sophygoapp/frontend-client.log`                     | Файл логов frontend, присланных браузером              |
| `CLIENT_LOG_MAX_BODY_BYTES` | `65536`                                                  | Максимальный размер пакета клиентских логов            |
| `CLIENT_LOG_RATE_PER_MINUTE` | `120`                                                   | Лимит пакетов логов с одного IP в минуту               |
| `HTTP_ADDR`                | `:8080`                                                   | Адрес HTTP-сервера                                     |
| `EVENTS_FILE`              | `data/events.jsonl`                                       | JSONL-файл событий                                     |
| `CORS_ORIGINS`             | `http://localhost:5173,http://127.0.0.1:5173`             | Разрешённые Origin                                     |
| `MAX_RECENT_EVENTS`        | `1000`                                                    | Размер буфера последних событий                        |
| `SHUTDOWN_TIMEOUT`         | `10s`                                                     | Ожидание завершения запросов                           |
| `HTTP_MAX_BODY_BYTES`      | `65536`                                                   | Максимальный размер тела запроса                       |
| `HTTP_REQUEST_TIMEOUT`     | `15s`                                                     | Таймаут HTTP-обработчиков                              |
| `HTTP_READ_HEADER_TIMEOUT` | `5s`                                                      | Таймаут чтения заголовков                              |
| `HTTP_READ_TIMEOUT`        | `15s`                                                     | Таймаут чтения запроса                                 |
| `HTTP_WRITE_TIMEOUT`       | `20s`                                                     | Таймаут записи ответа                                  |
| `HTTP_IDLE_TIMEOUT`        | `60s`                                                     | Таймаут idle-соединения                                |
| `EVENTS_DEFAULT_LIMIT`     | `50`                                                      | Лимит событий по умолчанию                             |
| `EVENTS_MAX_LIMIT`         | `500`                                                     | Максимальный лимит событий                             |
| `WS_MAX_MESSAGE_BYTES`     | `1048576`                                                 | Максимальный размер WebSocket-сообщения                |
| `WS_PING_INTERVAL`         | `20s`                                                     | Период ping WebSocket                                  |
| `WS_PING_TIMEOUT`          | `10s`                                                     | Таймаут ping WebSocket                                 |
| `WS_WRITE_TIMEOUT`         | `10s`                                                     | Таймаут записи в WebSocket                             |
| `REALTIME_MAX_READ_BYTES`  | `16777216`                                                | Максимальный размер события Realtime                   |
| `REALTIME_DIAL_TIMEOUT`    | `15s`                                                     | Таймаут подключения к Realtime                         |
| `REALTIME_WRITE_TIMEOUT`   | `10s`                                                     | Таймаут записи в Realtime                              |
| `GUIDE_CLEANUP_INTERVAL`   | `30s`                                                     | Период очистки истёкших сессий                         |
| `EVENT_MAX_PLACE_ID_LEN`   | `128`                                                     | Макс. длина `place.id`                                 |
| `EVENT_MAX_NAME_LEN`       | `300`                                                     | Макс. длина `place.name`                               |
| `EVENT_MAX_CATEGORY_LEN`   | `64`                                                      | Макс. длина `place.category`                           |
| `EVENT_MAX_SESSION_ID_LEN` | `128`                                                     | Макс. длина `client.sessionId`                         |
| `EVENT_MAX_USER_AGENT_LEN` | `512`                                                     | Макс. длина `userAgent`                                |
| `EVENT_MAX_LANGUAGE_LEN`   | `32`                                                      | Макс. длина `language`                                 |
| `EVENT_MAX_PLATFORM_LEN`   | `32`                                                      | Макс. длина `platform`                                 |
| `EVENT_MAX_REMOTE_IP_LEN`  | `64`                                                      | Макс. длина `remoteIp`                                 |
| `EVENT_MAX_CLOCK_SKEW`     | `24h`                                                     | Допустимое будущее время события                       |
| `EVENT_MAX_AGE`            | `720h`                                                    | Максимальный возраст события                           |
| `TELEGRAM_BOT_TOKEN`       | —                                                         | Токен бота для проверки initData                       |
| `TELEGRAM_INIT_DATA_MAX_AGE` | `24h`                                                   | Максимальный возраст initData                          |
| `DEV_AUTH_USER_ID`         | `0`                                                       | Dev-авторизация без initData (0 — выключено)           |
| `YANDEX_API_KEY`           | —                                                         | API-ключ Yandex Cloud                                  |
| `YANDEX_FOLDER_ID`         | —                                                         | Folder id Yandex Cloud                                 |
| `YANDEX_REALTIME_URL`      | `wss://ai.api.cloud.yandex.net/v1/realtime`               | WebSocket endpoint                                     |
| `YANDEX_REALTIME_MODEL`    | `gpt://<folder>/speech-realtime-deepseek-v4-flash`        | modelUri голосовой модели                              |
| `YANDEX_INPUT_RATE`        | `16000`                                                   | Частота PCM на входе                                   |
| `YANDEX_OUTPUT_RATE`       | `16000`                                                   | Частота PCM на выходе                                  |
| `YANDEX_VOICE`             | `alena`                                                   | Голос Гида                                             |
| `YANDEX_VOICE_ROLE`        | `good`                                                    | Эмоциональная роль голоса                              |
| `YANDEX_VAD_THRESHOLD`     | `0.5`                                                     | Чувствительность VAD                                   |
| `YANDEX_VAD_SILENCE_MS`    | `400`                                                     | Тишина для завершения реплики                          |
| `GUIDE_INSTRUCTIONS`       | системный промпт Гида                                     | Инструкции модели                                      |
| `SESSION_TTL`              | `2h`                                                      | Время жизни сессии                                     |
| `REALTIME_TOKEN_TTL`       | `60s`                                                     | Время жизни одноразового токена WebSocket              |

## API

### `POST /api/v1/session`

Заголовок: `X-Telegram-Init-Data: <initData>`.

Проверяет подпись, создаёт или переиспользует сессию пользователя и выдаёт
одноразовый токен для WebSocket.

```json
{
  "sessionId": "sess_...",
  "realtimeToken": "rt_...",
  "realtimePath": "/api/v1/realtime",
  "realtimeAvailable": true,
  "expiresAt": "2026-11-19T22:00:00Z",
  "user": { "id": 279058397, "username": "vlad", "languageCode": "ru" }
}
```

### `GET /api/v1/realtime?token=<realtimeToken>`

WebSocket-соединение. Токен одноразовый и живёт `REALTIME_TOKEN_TTL`.
Сервер сам подключается к Yandex Realtime API с системным промптом и
параметрами аудио.

От браузера принимаются только события:

- `input_audio_buffer.append` — PCM16 mono в base64;
- `input_audio_buffer.commit`;
- `response.cancel`.

Попытки отправить `session.update` или другие управляющие события
игнорируются, чтобы клиент не мог изменить системный промпт.

### Формат аудио

Yandex Realtime API документирует только `audio/pcm`; сжатые G.711 и Opus
в официальных примерах и поиске по документации не поддерживаются. Поэтому
используется PCM16 mono:

- по умолчанию 16 кГц — wideband-речь, разборчивая для STT;
- сырой поток — около 32 КБ/с, после base64 — около 42,7 КБ/с;
- для сравнения, 44,1 кГц даёт 88,2 КБ/с сырых данных.

Base64 кодирование и декодирование выполняет frontend. Go-сервис — простой
ретранслятор: он не перекодирует и не изменяет аудио, а только проверяет
сессию и пересылает JSON между браузером и Yandex. Частоты из
`YANDEX_INPUT_RATE`/`YANDEX_OUTPUT_RATE` возвращаются в ответе
`POST /api/v1/session`, чтобы клиент использовал ровно тот же формат.

Если конкретная модель Realtime не примет 16 кГц, задайте `44100` в обоих
переменных — frontend подхватит частоту из ответа сессии.

Основные события от сервера:

- `response.output_audio.delta` — base64 PCM16 для воспроизведения;
- `response.output_text.delta` — текст ответа;
- `conversation.item.input_audio_transcription.completed` — распознанная речь;
- `input_audio_buffer.speech_started` — пользователь начал говорить (перебивание);
- `response.done`, `error`.

### `POST /api/v1/events/place-open`

Заголовок `X-Telegram-Init-Data` необязателен. Без него событие сохраняется
анонимно. С ним сервер проверяет пользователя и отправляет промпт в его
Realtime-сессию.

```json
{
  "place": {
    "id": "way/123",
    "name": "Красная площадь",
    "category": "historic_building",
    "latitude": 55.7539,
    "longitude": 37.6208
  },
  "client": {
    "sessionId": "2f1c9a4e-...",
    "platform": "telegram",
    "userAgent": "Mozilla/5.0 ...",
    "language": "ru-RU"
  },
  "sessionId": "sess_...",
  "occurredAt": "2026-11-19T20:00:00Z"
}
```

Ответ `202 Accepted`:

```json
{
  "id": "evt_...",
  "receivedAt": "2026-11-19T20:10:11Z",
  "sessionId": "sess_...",
  "delivered": true
}
```

`delivered: false` означает, что событие сохранено, но активного
WebSocket-соединения не было и промпт в Realtime-сессию не отправлен.

### `POST /api/v1/session/{sessionId}/stop`

Заголовок `X-Telegram-Init-Data` обязателен. Отправляет `response.cancel`
в Realtime-сессию. Ответ `204 No Content`. Если соединения нет — `409`.

### `GET /api/v1/events?limit=50`

Последние события из памяти, от новых к старым. Только для отладки.

### `GET /healthz`

Проверка работоспособности.

## Проверка Telegram initData

Подпись проверяется по алгоритму Telegram:

```text
secret_key = HMAC_SHA256(key=bot_token, message="WebAppData")
hash       = HMAC_SHA256(key=secret_key, message=data_check_string)
```

`data_check_string` — отсортированные по алфавиту пары `key=value`, кроме
`hash`. Дополнительно проверяется `auth_date`. `initDataUnsafe` на сервере
не используется.

## Хранение событий

События дописываются в JSONL-файл (`EVENTS_FILE`), по одному объекту на
строку:

```bash
tail -n 20 backend/data/events.jsonl
```

Последние события дублируются в памяти для `GET /api/v1/events`. Сессии
Гида хранятся в памяти и теряются при перезапуске сервиса.

## Тесты и проверки

```bash
cd backend
make test
make vet
```

Тесты покрывают проверку initData, клиент Realtime API, менеджер сессий,
HTTP-обработчики и WebSocket-прокси (с фейковым Yandex Realtime-сервером).

Интеграционный тест открывает реальную сессию Yandex Realtime и проверяет
доступность модели. Настройки и ключи он берёт из `.env`; без ключей тест
пропускается:

```bash
cd backend
go test ./internal/realtime -run YandexRealtimeSession -count=1 -v
```

## Развёртывание на VPS

Сборка:

```bash
cd backend
make build
# бинарник: backend/bin/sophygo-api
```

Пример unit-файла systemd (`/etc/systemd/system/sophygo-api.service`):

```ini
[Unit]
Description=SophyGo API
After=network.target

[Service]
User=sophygo
WorkingDirectory=/opt/sophygo/backend
ExecStart=/opt/sophygo/backend/bin/sophygo-api
EnvironmentFile=/opt/sophygo/backend/.env
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Для WebSocket обязателен HTTPS/WSS. Пример nginx:

```nginx
location /api/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

`proxy_read_timeout` должен быть больше времени жизни WebSocket-сессии,
иначе nginx разорвёт соединение во время рассказа Гида.

## Интеграция с frontend

- [`frontend/index.html`](../frontend/index.html) подключает Telegram WebApp SDK.
- [`frontend/src/platform/telegram.ts`](../frontend/src/platform/telegram.ts) — адаптер Telegram.
- [`frontend/src/api/guide.ts`](../frontend/src/api/guide.ts) — создание сессии и WebSocket URL.
- [`frontend/src/api/events.ts`](../frontend/src/api/events.ts) — отправка объекта с `initData`.
- [`frontend/src/audio/audioEngine.ts`](../frontend/src/audio/audioEngine.ts) — захват и воспроизведение PCM16 16 кГц, base64 на клиенте.
- [`frontend/src/audio/sileroVad.ts`](../frontend/src/audio/sileroVad.ts) — Silero VAD (ONNX/WASM).
- [`frontend/src/guide/guideClient.ts`](../frontend/src/guide/guideClient.ts) — жизненный цикл голосовой сессии.

В dev-режиме Vite проксирует HTTP и WebSocket `/api` на `http://localhost:8080`
(см. [`frontend/vite.config.ts`](../frontend/vite.config.ts)). На VPS ту же роль
выполняет nginx.

### Детекция речи (VAD)

В Mini App работают два VAD:

1. **Silero VAD v5 на устройстве** через ONNX Runtime Web (WASM). Модель
   загружается лениво при первом включении микрофона. Она определяет речь
   пользователя и позволяет не отправлять тишину и эхо динамика, пока Гид
   говорит. Голос пользователя при этом уходит сразу — так работает
   перебивание.
2. **Server VAD Яндекса** (`turn_detection: server_vad`) определяет границы
   реплик и завершает ход пользователя.

Пока Silero VAD загружается, используется простой энергетический VAD по RMS.
Если модель не загрузилась (нет сети, CSP, старый WebView), приложение
продолжает работать на RMS-fallback.

Если пользователь молчит 45 секунд (и Гид в этот момент не говорит),
микрофон выключается и устройство освобождается. Продолжить можно кнопкой
«Говорить с Гидом».

Модель Silero (~2,3 МБ) и wasm ONNX Runtime (~15 МБ) лежат в
`frontend/public/vad/` и `frontend/public/ort/` и отдаются с того же Origin.
Это надёжнее CDN внутри Telegram WebView. Обновить файлы можно командой:

```bash
cd frontend
npm run vad:assets
```

Переключение на CDN (необязательно):

```bash
VITE_VAD_ASSET_BASE_URL=https://cdn.jsdelivr.net/npm/@ricky0123/vad-web@0.0.31/dist/
VITE_ORT_ASSET_BASE_URL=https://cdn.jsdelivr.net/npm/onnxruntime-web@1.31.1/dist/
```

Оба пути должны заканчиваться на `/`. При собственном CSP разрешите
`wasm-unsafe-eval` в `script-src`, а при загрузке с CDN — `cdn.jsdelivr.net`
в `connect-src`. Для AudioWorklet может потребоваться `worker-src blob:`.

Для nginx важно отдавать `.wasm` с MIME `application/wasm`, включить gzip
и кэширование:

```nginx
types {
    application/wasm wasm;
}

gzip on;
gzip_types application/wasm application/octet-stream;

location /vad/ {
    expires 7d;
    add_header Cache-Control "public, max-age=604800";
}

location /ort/ {
    expires 7d;
    add_header Cache-Control "public, max-age=604800";
}
```

Первый запуск микрофона скачивает ~17 МБ, дальше файлы берутся из кэша.
Silero VAD работает только пока микрофон включён, обрабатывает кадры по
32 мс в один поток WASM и не блокирует интерфейс. Если кадры VAD перестают
приходить, приложение автоматически возвращается к RMS-fallback.

Рекомендуется использовать наушники: эхо динамика может восприниматься
как речь и вызывать ложное перебивание.

## Ограничения текущего этапа

- Нет авторизации, кроме проверки Telegram `initData`.
- Нет rate limiting и квот на пользователя.
- Сессии и одноразовые токены хранятся в памяти: при перезапуске сервиса
  активные голосовые сессии закрываются.
- На одного пользователя — одна активная Realtime-сессия; новое подключение
  закрывает предыдущее.
- Нет PostgreSQL/PostGIS: события пишутся в JSONL.
- Ротацию файла событий нужно настроить отдельно (например, через logrotate).
- `GET /api/v1/events` предназначен для отладки, а не для публичного доступа.
- Для работы микрофона и звука нужен HTTPS и разрешение пользователя.
