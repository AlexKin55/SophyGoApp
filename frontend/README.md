# React + TypeScript + Vite

This template provides a minimal setup to get React working in Vite with HMR and some Oxlint rules.

## Параметры окружения

Все таймауты, лимиты, адреса внешних API и настройки карты вынесены в
`VITE_*`-переменные. Полный список с комментариями — в `.env.example`;
чтение и значения по умолчанию собраны в `src/config/env.ts`.

```bash
cd frontend
cp .env.example .env.local
npm run dev
```

Для локальной отладки без Telegram создайте `.env.development.local`
(применяется только в dev-режиме, не попадает в production build):

```env
VITE_DEV_AUTH=true
```

На backend при этом должен быть задан `DEV_AUTH_USER_ID`.

## Логирование

Логирование выключено по умолчанию (`VITE_LOGGING_ENABLED=false`).
При включении записи пишутся в консоль и отправляются пакетами на backend
(`POST /api/v1/client-logs`), где сохраняются в файл из `CLIENT_LOG_FILE`
(по умолчанию `/tmp/sophygoapp/frontend-client.log`). В Telegram WebView
это единственный способ сохранить логи frontend на сервере.

```env
VITE_LOGGING_ENABLED=true
VITE_LOG_LEVEL=debug
VITE_CLIENT_LOG_ENDPOINT=/api/v1/client-logs
VITE_CLIENT_LOG_BATCH_SIZE=20
VITE_CLIENT_LOG_FLUSH_MS=1000
```

Вывод Vite dev-сервера (`npm run dev`) скрипт `scripts/start_services.py`
перенаправляет в `LOG_FILE` из `.env` (по умолчанию
`/tmp/sophygoapp/frontend.log`).

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Oxc](https://oxc.rs)
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/)

## React Compiler

The React Compiler is not enabled on this template because of its impact on dev & build performances. To add it, see [this documentation](https://react.dev/learn/react-compiler/installation).

## Expanding the Oxlint configuration

If you are developing a production application, we recommend enabling type-aware lint rules by installing `oxlint-tsgolint` and editing `.oxlintrc.json`:

```json
{
  "$schema": "./node_modules/oxlint/configuration_schema.json",
  "plugins": ["react", "typescript", "oxc"],
  "options": {
    "typeAware": true
  },
  "rules": {
    "react/rules-of-hooks": "error",
    "react/only-export-components": ["warn", { "allowConstantExport": true }]
  }
}
```

See the [Oxlint rules documentation](https://oxc.rs/docs/guide/usage/linter/rules) for the full list of rules and categories.
