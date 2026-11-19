#!/usr/bin/env python3
"""Запуск тестов и проверок SophyGo.

Использование:
    python3 scripts/run_tests.py                 # backend + frontend
    python3 scripts/run_tests.py --target backend
    python3 scripts/run_tests.py --target frontend
    python3 scripts/run_tests.py --skip-integration
    python3 scripts/run_tests.py --verbose

Backend: gofmt-проверка, go vet, go test.
Frontend: npm run lint, npm run build (включает tsc).

Интеграционный тест Yandex Realtime берёт ключи из backend/.env.
Флаг --skip-integration отключает его через SOPHYGO_SKIP_INTEGRATION=1.
"""

from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
from pathlib import Path

from _common import BACKEND_DIR, FRONTEND_DIR, build_env, fail, log, require_tool


def run_step(
    name: str,
    command: list[str],
    cwd: Path,
    env: dict[str, str] | None = None,
    timeout: float | None = None,
) -> None:
    log(f"{name}: {' '.join(command)}")
    try:
        result = subprocess.run(
            command,
            cwd=str(cwd),
            env=env or build_env(),
            timeout=timeout,
            check=False,
        )
    except FileNotFoundError:
        fail(f"{name}: команда не найдена: {command[0]}")
    except subprocess.TimeoutExpired:
        fail(f"{name}: превышен таймаут {timeout:.0f} с")

    if result.returncode != 0:
        fail(f"{name} завершился с кодом {result.returncode}")
    log(f"{name}: OK")


def check_gofmt() -> None:
    env = build_env()
    gofmt = shutil.which("gofmt", path=env.get("PATH"))
    if not gofmt:
        go = require_tool("go")
        gofmt = str(Path(go).with_name("gofmt"))

    log("gofmt: проверка форматирования")
    result = subprocess.run(
        [gofmt, "-l", "."],
        cwd=str(BACKEND_DIR),
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        fail(f"gofmt завершился с кодом {result.returncode}: {result.stderr.strip()}")

    unformatted = [line for line in result.stdout.splitlines() if line.strip()]
    if unformatted:
        fail(
            "файлы не отформатированы (запустите `gofmt -w backend`):\n  "
            + "\n  ".join(unformatted)
        )
    log("gofmt: OK")


def run_backend_tests(skip_integration: bool, verbose: bool) -> None:
    go = require_tool("go")

    check_gofmt()
    run_step("go vet", [go, "vet", "./..."], BACKEND_DIR)

    env = build_env()
    if skip_integration:
        env["SOPHYGO_SKIP_INTEGRATION"] = "1"

    command = [go, "test", "./...", "-count=1"]
    if verbose:
        command.append("-v")
    run_step("go test", command, BACKEND_DIR, env=env)


def run_frontend_tests() -> None:
    npm = require_tool("npm")
    run_step("frontend lint", [npm, "run", "lint"], FRONTEND_DIR)
    run_step("frontend build", [npm, "run", "build"], FRONTEND_DIR)


def main() -> int:
    parser = argparse.ArgumentParser(description="Тесты и проверки SophyGo")
    parser.add_argument(
        "--target",
        choices=("all", "backend", "frontend"),
        default="all",
        help="что проверять (по умолчанию всё)",
    )
    parser.add_argument(
        "--skip-integration",
        action="store_true",
        help="не запускать интеграционный тест Yandex Realtime",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="подробный вывод go test (-v)",
    )
    args = parser.parse_args()

    if args.target in ("all", "backend"):
        run_backend_tests(args.skip_integration, args.verbose)

    if args.target in ("all", "frontend"):
        run_frontend_tests()

    log("все проверки пройдены")
    return 0


if __name__ == "__main__":
    sys.exit(main())
