#!/usr/bin/env python3
"""Сборка MAX-сборки SophyGo и публикация образа в Yandex Container Registry.

Авторизация в реестре выполняется клиентом yc (docker credential helper),
поэтому отдельный docker login и key.json не нужны.

Использование:
    python3 SophyGoContainer/deploy.py
    TAG=v2 python3 SophyGoContainer/deploy.py
    DEPLOY_REVISION=1 YC_SERVICE_ACCOUNT_ID=<id> python3 SophyGoContainer/deploy.py

Переменные окружения:
    PROJECT_DIR             — корень проекта (по умолчанию каталог над SophyGoContainer)
    REGISTRY                — реестр (по умолчанию cr.yandex/crp7hfn5k6uh54cfia4q)
    IMAGE_NAME              — имя образа (по умолчанию sophygo)
    TAG                     — тег (по умолчанию v1)
    PLATFORM                — платформа сборки (по умолчанию linux/amd64)
    CONTAINER_ID            — id serverless-контейнера (по умолчанию bba7hk3pi4suknk58vuu)
    DEPLOY_REVISION         — 1, чтобы сразу развернуть ревизию через yc
    YC_SERVICE_ACCOUNT_ID   — сервисный аккаунт для деплоя ревизии
    CONTAINER_CORES         — CPU ревизии (по умолчанию 1)
    CONTAINER_MEMORY        — память ревизии (по умолчанию 512MB)
    CONTAINER_CONCURRENCY   — конкурентность ревизии (по умолчанию 8)
    CONTAINER_TIMEOUT       — execution timeout ревизии (по умолчанию 15s)
"""

from __future__ import annotations

import argparse
import os
import shlex
import shutil
import subprocess
import sys
from pathlib import Path
from typing import NoReturn

SCRIPT_DIR = Path(__file__).resolve().parent
# Dockerfile лежит в SophyGoContainer/, контекст сборки — корень проекта.
DEFAULT_PROJECT_DIR = SCRIPT_DIR.parent
DEFAULT_REGISTRY = "cr.yandex/crp7hfn5k6uh54cfia4q"
DEFAULT_IMAGE_NAME = "sophygo"
DEFAULT_TAG = "v1"
DEFAULT_PLATFORM = "linux/amd64"
DEFAULT_CONTAINER_ID = "bba7hk3pi4suknk58vuu"


def log(message: str) -> None:
    print(f"[sophygo-container] {message}", flush=True)


def fail(message: str, code: int = 1) -> NoReturn:
    print(f"[sophygo-container] ОШИБКА: {message}", file=sys.stderr, flush=True)
    raise SystemExit(code)


def run(command: list[str]) -> None:
    log("$ " + " ".join(shlex.quote(part) for part in command))
    try:
        subprocess.run(command, check=True)
    except FileNotFoundError:
        fail(f"не найден исполняемый файл: {command[0]}")
    except subprocess.CalledProcessError as error:
        fail(f"команда завершилась с кодом {error.returncode}: {command[0]}")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Сборка и публикация образа SophyGo для Yandex Serverless Containers",
    )
    parser.add_argument(
        "--project-dir",
        type=Path,
        default=Path(os.environ.get("PROJECT_DIR", DEFAULT_PROJECT_DIR)),
        help=f"корень проекта (по умолчанию {DEFAULT_PROJECT_DIR})",
    )
    parser.add_argument(
        "--tag",
        default=os.environ.get("TAG", DEFAULT_TAG),
        help=f"тег образа (по умолчанию {DEFAULT_TAG})",
    )
    parser.add_argument(
        "--platform",
        default=os.environ.get("PLATFORM", DEFAULT_PLATFORM),
        help=f"платформа сборки (по умолчанию {DEFAULT_PLATFORM})",
    )
    parser.add_argument(
        "--deploy-revision",
        action="store_true",
        default=os.environ.get("DEPLOY_REVISION", "0") == "1",
        help="развернуть ревизию serverless-контейнера после публикации",
    )
    parser.add_argument(
        "--skip-configure-docker",
        action="store_true",
        help="не вызывать yc container registry configure-docker",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()

    project_dir: Path = args.project_dir.expanduser().resolve()
    dockerfile = SCRIPT_DIR / "Dockerfile"
    registry = os.environ.get("REGISTRY", DEFAULT_REGISTRY).rstrip("/")
    image_name = os.environ.get("IMAGE_NAME", DEFAULT_IMAGE_NAME)
    image = f"{registry}/{image_name}:{args.tag}"
    container_id = os.environ.get("CONTAINER_ID", DEFAULT_CONTAINER_ID)

    if shutil.which("docker") is None:
        fail("не найден docker")
    if shutil.which("yc") is None:
        fail("не найден yc")
    if not project_dir.is_dir():
        fail(f"не найден каталог проекта: {project_dir}")
    if not dockerfile.is_file():
        fail(f"не найден Dockerfile: {dockerfile}")

    if not args.skip_configure_docker:
        log("настройка docker для Yandex Container Registry через yc")
        run(["yc", "container", "registry", "configure-docker"])

    log(f"сборка образа {image} ({args.platform})")
    run([
        "docker", "build",
        "--platform", args.platform,
        # Yandex Container Registry не принимает OCI index с attestation-
        # манифестами, поэтому отключаем provenance и SBOM.
        "--provenance=false",
        "--sbom=false",
        "-f", str(dockerfile),
        "-t", image,
        str(project_dir),
    ])

    log(f"публикация образа {image}")
    run(["docker", "push", image])

    if args.deploy_revision:
        service_account_id = os.environ.get("YC_SERVICE_ACCOUNT_ID", "").strip()
        if not service_account_id:
            fail("для деплоя ревизии задайте YC_SERVICE_ACCOUNT_ID")

        log(f"деплой ревизии контейнера {container_id}")
        run([
            "yc", "serverless", "container", "revision", "deploy",
            "--container-id", container_id,
            "--image", image,
            "--platform", args.platform,
            "--service-account-id", service_account_id,
            "--cores", os.environ.get("CONTAINER_CORES", "1"),
            "--memory", os.environ.get("CONTAINER_MEMORY", "512MB"),
            "--concurrency", os.environ.get("CONTAINER_CONCURRENCY", "8"),
            "--execution-timeout", os.environ.get("CONTAINER_TIMEOUT", "15s"),
        ])

    log(f"готово: {image}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
