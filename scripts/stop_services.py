#!/usr/bin/env python3
"""Остановка backend и frontend SophyGo.

Использование:
    python3 scripts/stop_services.py
    python3 scripts/stop_services.py --only backend
    python3 scripts/stop_services.py --only frontend

Скрипт использует PID-файлы из scripts/.run/ и дополнительно проверяет
процессы, слушающие порты 8080 и 5173 (только процессы этого проекта).
"""

from __future__ import annotations

import argparse
import sys

from _common import SERVICE_FILES, log, pids_on_port, stop_service


def main() -> int:
    parser = argparse.ArgumentParser(description="Остановка сервисов SophyGo")
    parser.add_argument(
        "--only",
        choices=("backend", "frontend", "both"),
        default="both",
        help="какой сервис останавливать (по умолчанию оба)",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=10.0,
        help="сколько ждать завершения каждого сервиса, секунд",
    )
    args = parser.parse_args()

    targets = ["backend", "frontend"] if args.only == "both" else [args.only]

    for name in targets:
        stop_service(name, timeout=args.timeout)

    for name in targets:
        port = SERVICE_FILES[name]["port"]
        remaining = pids_on_port(port)
        if remaining:
            log(f"порт {port} всё ещё занят процессами: {remaining}")
        else:
            log(f"порт {port} свободен")

    return 0


if __name__ == "__main__":
    sys.exit(main())
