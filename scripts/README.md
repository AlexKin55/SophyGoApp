# Скрипты SophyGo

Python 3, только стандартная библиотека. Запускать из корня проекта или из любой папки — пути вычисляются относительно скриптов.

| Скрипт | Назначение |
| --- | --- |
| `build_backend.py` | Собирает backend в `backend/bin/sophygo-api` |
| `start_services.py` | Запускает backend и frontend в фоне, пишет PID и логи |
| `stop_services.py` | Останавливает backend и frontend |
| `run_tests.py` | Запускает проверки и тесты backend и frontend |

## Примеры

```bash
# Сборка backend
python3 scripts/build_backend.py

# Запуск обоих сервисов
python3 scripts/start_services.py

# Только backend или только frontend
python3 scripts/start_services.py --only backend
python3 scripts/start_services.py --only frontend

# Перезапуск
python3 scripts/start_services.py --restart

# Остановка
python3 scripts/stop_services.py
python3 scripts/stop_services.py --only frontend

# Все проверки (backend + frontend)
python3 scripts/run_tests.py

# Только backend или только frontend
python3 scripts/run_tests.py --target backend
python3 scripts/run_tests.py --target frontend

# Без сетевого интеграционного теста Yandex Realtime
python3 scripts/run_tests.py --skip-integration
```

## Что важно знать

- PID-файлы лежат в `scripts/.run/`, логи — по путям из `.env` сервисов.
  По умолчанию: `/tmp/sophygoapp/backend.log`, `/tmp/sophygoapp/frontend.log`
  (вывод Vite) и `/tmp/sophygoapp/frontend-client.log` (логи браузера,
  их принимает backend).
- `start_services.py` читает `LOG_FILE` из `backend/.env` и `frontend/.env*`;
  каталог логов создаётся автоматически.
- Backend запускается через `go run ./cmd/server` из `backend/`, поэтому сам читает `backend/.env`.
- Frontend запускается через `npm run dev -- --port 5173 --strictPort` из `frontend/` и подхватывает `frontend/.env*`.
- `stop_services.py` сначала использует PID-файлы, а если их нет — ищет процессы проекта на портах `8080` и `5173`.
- Останавливаются только процессы этого проекта: проверяется рабочий каталог и командная строка.
- `run_tests.py` для backend выполняет проверку `gofmt`, `go vet` и `go test`; для frontend — `npm run lint` и `npm run build`.
- Интеграционный тест Yandex Realtime берёт ключи из `backend/.env`; флаг `--skip-integration` отключает его.
- `scripts/.run/` стоит добавить в `.gitignore`.
