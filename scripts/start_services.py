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
import sys

from _common import (
    BACKEND_DIR,
    FRONTEND_DIR,
    FRONTEND_PORT,
    HEALTH_URLS,
    fail,
    log,
    require_tool,
    service_log_path,
    start_service,
    stop_service,
)


def backend_command() -> list[str]:
    go = require_tool("go")
    return [go, "run", "./cmd/server"]


def frontend_command() -> list[str]:
    npm = require_tool("npm")
    return [npm, "run", "dev", "--", "--port", str(FRONTEND_PORT), "--strictPort"]


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
    args = parser.parse_args()

    targets = ["backend", "frontend"] if args.only == "both" else [args.only]

    if "backend" in targets:
        env_file = BACKEND_DIR / ".env"
        if not env_file.exists():
            log(
                "предупреждение: backend/.env не найден. "
                "Скопируйте backend/.env.example в backend/.env и заполните ключи."
            )

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
