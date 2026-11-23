#!/usr/bin/env python3
"""Запуск backend и frontend SophyGo в фоне.

Использование:
    python3 scripts/start_services.py
    python3 scripts/start_services.py --only backend
    python3 scripts/start_services.py --only frontend
    python3 scripts/start_services.py --restart

PID-файлы и логи складываются в scripts/.run/.
Backend читает backend/.env автоматически; frontend — frontend/.env*.
"""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys

from _common import (
    BACKEND_BINARY,
    BACKEND_DIR,
    FRONTEND_DIR,
    FRONTEND_PORT,
    HEALTH_URLS,
    build_env,
    fail,
    log,
    require_tool,
    service_log_path,
    start_service,
    stop_service,
)


def backend_command() -> list[str]:
    """go run, если Go установлен, иначе — заранее собранный бинарник."""
    go = shutil.which("go", path=build_env().get("PATH"))
    if go:
        return [go, "run", "./cmd/server"]
    if BACKEND_BINARY.is_file():
        return [str(BACKEND_BINARY)]
    fail("не найден go и нет собранного backend/bin/sophygo-api")
    return []  # недостижимо


def frontend_command() -> list[str]:
    npm = require_tool("npm")
    command = [npm, "run", "dev", "--", "--port", str(FRONTEND_PORT), "--strictPort"]
    host = os.environ.get("FRONTEND_HOST", "").strip()
    if host:
        command.extend(["--host", host])
    return command


def set_env_value(content: str, key: str, value: str) -> str:
    """Заменяет или добавляет KEY=VALUE в содержимом .env-файла."""
    lines = content.splitlines()
    prefix = f"{key}="
    for index, line in enumerate(lines):
        if line.strip().startswith(prefix):
            lines[index] = f"{key}={value}"
            break
    else:
        lines.append(f"{key}={value}")

    return "\n".join(lines) + "\n"


def ensure_backend_env() -> None:
    """Создаёт backend/.env для локального запуска, если его ещё нет."""
    env_path = BACKEND_DIR / ".env"
    if env_path.exists():
        return

    example = BACKEND_DIR / ".env.example"
    if not example.exists():
        log("предупреждение: backend/.env и backend/.env.example не найдены")
        return

    content = example.read_text(encoding="utf-8")
    content = set_env_value(
        content,
        "CORS_ORIGINS",
        "http://localhost:5173,http://127.0.0.1:5173",
    )
    content = set_env_value(content, "DEV_AUTH_USER_ID", "1")
    env_path.write_text(content, encoding="utf-8")

    log("создан backend/.env из backend/.env.example (локальный режим)")
    log(
        "для голосового Гида заполните TELEGRAM_BOT_TOKEN, "
        "YANDEX_API_KEY и YANDEX_FOLDER_ID"
    )


def ensure_frontend_env() -> None:
    """Включает локальную отладку без Telegram в dev-режиме."""
    env_path = FRONTEND_DIR / ".env.development.local"
    if env_path.exists():
        return

    env_path.write_text("VITE_DEV_AUTH=true\n", encoding="utf-8")
    log("создан frontend/.env.development.local (VITE_DEV_AUTH=true)")


def ensure_frontend_deps() -> None:
    """Устанавливает npm-зависимости, если после клонирования их ещё нет."""
    if (FRONTEND_DIR / "node_modules").is_dir():
        return

    npm = require_tool("npm")
    log("устанавливаю зависимости frontend (npm ci)…")
    result = subprocess.run(
        [npm, "ci", "--no-audit", "--no-fund"],
        cwd=FRONTEND_DIR,
        check=False,
    )
    if result.returncode != 0:
        fail("не удалось установить зависимости frontend (npm ci)")


def main() -> int:
    parser = argparse.ArgumentParser(description="Запуск сервисов SophyGo")
    parser.add_argument(
        "--only",
        choices=("backend", "frontend", "both"),
        default="both",
        help="какой сервис запускать (по умолчанию оба)",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=30.0,
        help="сколько ждать health-check, секунд (по умолчанию 30)",
    )
    parser.add_argument(
        "--no-wait",
        action="store_true",
        help="не ждать ответа сервисов после запуска",
    )
    parser.add_argument(
        "--restart",
        action="store_true",
        help="сначала остановить выбранные сервисы",
    )
    parser.add_argument(
        "--foreground",
        action="store_true",
        help="запустить один сервис на месте процесса (для Docker/контейнеров)",
    )
    args = parser.parse_args()

    targets = ["backend", "frontend"] if args.only == "both" else [args.only]

    if args.foreground:
        if len(targets) != 1:
            fail("--foreground требует ровно один сервис: используйте --only backend")
        name = targets[0]
        command = backend_command() if name == "backend" else frontend_command()
        cwd = BACKEND_DIR if name == "backend" else FRONTEND_DIR
        log(f"запуск {name} в foreground: {' '.join(command)}")
        os.chdir(cwd)
        # Заменяем процесс Python на сервис: PID 1 и сигналы остаются корректными.
        os.execvpe(command[0], command, build_env())
        return 0  # недостижимо

    if "backend" in targets:
        ensure_backend_env()

    if "frontend" in targets:
        ensure_frontend_env()
        ensure_frontend_deps()

    if args.restart:
        for name in targets:
            stop_service(name)

    results: list[tuple[str, int | None]] = []

    if "backend" in targets:
        pid = start_service(
            "backend",
            backend_command(),
            BACKEND_DIR,
            timeout=args.timeout,
            wait=not args.no_wait,
        )
        results.append(("backend", pid))

    if "frontend" in targets:
        pid = start_service(
            "frontend",
            frontend_command(),
            FRONTEND_DIR,
            timeout=args.timeout,
            wait=not args.no_wait,
        )
        results.append(("frontend", pid))

    log("итог:")
    for name, pid in results:
        log(f"  {name}: pid={pid}, url={HEALTH_URLS[name]}, лог={service_log_path(name)}")

    if any(pid is None for _, pid in results):
        fail("не все сервисы удалось запустить")

    return 0


if __name__ == "__main__":
    sys.exit(main())
