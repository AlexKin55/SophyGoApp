#!/usr/bin/env python3
"""Сборка бинарника backend SophyGo.

Использование:
    python3 scripts/build_backend.py
    python3 scripts/build_backend.py --api=max
    python3 scripts/build_backend.py --output backend/bin/sophygo-api

--api выбирает мессенджер, под который собирается бинарник:
telegram (по умолчанию) или max. Значение вшивается в бинарник
через -ldflags "-X main.apiKind=...".

Требуется Go (по умолчанию /usr/local/go/bin добавляется в PATH).
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path

from _common import (
    BACKEND_BINARY,
    BACKEND_DIR,
    build_env,
    fail,
    log,
    require_tool,
)


def main() -> int:
    parser = argparse.ArgumentParser(description="Сборка backend SophyGo")
    parser.add_argument(
        "--api",
        choices=("telegram", "max"),
        default="telegram",
        help="мессенджер для сборки: telegram (по умолчанию) или max",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=BACKEND_BINARY,
        help=f"путь к бинарнику (по умолчанию {BACKEND_BINARY})",
    )
    parser.add_argument(
        "--no-trimpath",
        action="store_true",
        help="не использовать -trimpath",
    )
    args = parser.parse_args()

    go = require_tool("go")
    output = args.output if args.output.is_absolute() else (Path.cwd() / args.output)
    output.parent.mkdir(parents=True, exist_ok=True)

    command = [go, "build"]
    if not args.no_trimpath:
        command.append("-trimpath")
    command.extend([
        "-ldflags",
        f"-X main.apiKind={args.api}",
        "-o",
        str(output),
        "./cmd/server",
    ])

    log(f"сборка backend (api={args.api}): {' '.join(command)}")
    try:
        result = subprocess.run(command, cwd=str(BACKEND_DIR), env=build_env(), check=False)
    except OSError as error:
        fail(f"не удалось запустить go: {error}")

    if result.returncode != 0:
        fail(f"go build завершился с кодом {result.returncode}")

    size = output.stat().st_size if output.exists() else 0
    log(f"готово: {output} ({size / (1024 * 1024):.1f} МиБ)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
