#!/usr/bin/env python3
"""Деплой SophyGo на VPS.

Схема: сборка полностью локальная, на VPS загружаются только артефакты.
На VPS работают только systemd-сервис backend и nginx.

Использование:
    python3 deploy/deploy.py
    SUDO_PASS='<пароль sudo>' python3 deploy/deploy.py

Переменные окружения:
    VPS_HOST    — SSH-алиас VPS (по умолчанию SophyGo)
    REMOTE_DIR  — каталог приложения на VPS (по умолчанию /opt/sophygo)
    SUDO_PASS   — пароль sudo на VPS; если не задан, sudo спросит интерактивно
"""

from __future__ import annotations

import argparse
import os
import shlex
import subprocess
import sys
from pathlib import Path
from typing import NoReturn

ROOT_DIR = Path(__file__).resolve().parent.parent
DEFAULT_VPS_HOST = "SophyGo"
DEFAULT_REMOTE_DIR = "/opt/sophygo"


def log(message: str) -> None:
    print(f"[deploy] {message}", flush=True)


def fail(message: str, code: int = 1) -> NoReturn:
    print(f"[deploy] ОШИБКА: {message}", file=sys.stderr, flush=True)
    raise SystemExit(code)


def run(command: list[str], cwd: Path | None = None, stdin_text: str | None = None) -> None:
    display = " ".join(shlex.quote(part) for part in command)
    log(f"$ {display}")
    subprocess.run(
        command,
        cwd=str(cwd) if cwd else None,
        input=stdin_text,
        text=stdin_text is not None,
        check=True,
    )


def ssh(host: str, command: str, tty: bool = False, stdin_text: str | None = None) -> None:
    args = ["ssh"]
    if tty:
        args.append("-t")
    args.extend([host, command])
    run(args, stdin_text=stdin_text)


def run_sudo(host: str, commands: list[str], password: str) -> None:
    """Выполняет команды через sudo.

    При заданном пароле он передаётся в stdin и читается в переменную
    SUDO_PASS на VPS, поэтому не попадает ни в аргументы процессов, ни в лог.
    """
    if password:
        remote = "read -r SUDO_PASS; " + " && ".join(
            f'echo "$SUDO_PASS" | sudo -S {command}' for command in commands
        )
        ssh(host, remote, stdin_text=password + "\n")
        return

    ssh(host, " && ".join(f"sudo {command}" for command in commands), tty=True)


def main() -> int:
    parser = argparse.ArgumentParser(description="Деплой SophyGo на VPS")
    parser.add_argument(
        "--vps-host",
        default=os.environ.get("VPS_HOST", DEFAULT_VPS_HOST),
        help=f"SSH-алиас VPS (по умолчанию {DEFAULT_VPS_HOST})",
    )
    parser.add_argument(
        "--remote-dir",
        default=os.environ.get("REMOTE_DIR", DEFAULT_REMOTE_DIR),
        help=f"каталог приложения на VPS (по умолчанию {DEFAULT_REMOTE_DIR})",
    )
    parser.add_argument(
        "--skip-build",
        action="store_true",
        help="не собирать backend и frontend локально",
    )
    parser.add_argument(
        "--skip-upload",
        action="store_true",
        help="не загружать артефакты на VPS",
    )
    parser.add_argument(
        "--api",
        choices=("telegram", "max"),
        default="telegram",
        help="мессенджер, под который собирается backend (по умолчанию telegram)",
    )
    args = parser.parse_args()

    host = args.vps_host
    remote = args.remote_dir.rstrip("/")
    password = os.environ.get("SUDO_PASS", "")

    remote_backend = f"{remote}/backend"
    remote_bin = f"{remote_backend}/bin"
    remote_frontend = f"{remote}/frontend"
    remote_dist = f"{remote_frontend}/dist"
    remote_deploy = f"{remote}/deploy"
    remote_scripts = f"{remote}/scripts"
    remote_data = f"{remote}/data"

    try:
        if not args.skip_build:
            log(f"сборка backend (локально, api={args.api})")
            run([
                sys.executable,
                str(ROOT_DIR / "scripts" / "build_backend.py"),
                "--api",
                args.api,
            ])

            log("сборка frontend (локально)")
            run(["npm", "run", "build"], cwd=ROOT_DIR / "frontend")

        log(f"подготовка каталогов на {host}:{remote}")
        directories = " ".join(
            shlex.quote(path)
            for path in (remote_bin, remote_dist, remote_deploy, remote_scripts, remote_data)
        )
        ssh(host, f"mkdir -p {directories}")

        if not args.skip_upload:
            log("загрузка артефактов")
            run([
                "rsync", "-az",
                str(ROOT_DIR / "backend" / "bin" / "sophygo-api"),
                f"{host}:{remote_bin}/sophygo-api",
            ])
            run([
                "rsync", "-az", "--delete",
                f"{ROOT_DIR / 'frontend' / 'dist'}/",
                f"{host}:{remote_dist}/",
            ])
            run([
                "rsync", "-az", "--delete",
                f"{ROOT_DIR / 'deploy'}/",
                f"{host}:{remote_deploy}/",
            ])
            run([
                "rsync", "-az", "--delete",
                "--exclude", "__pycache__/",
                "--exclude", ".run/",
                f"{ROOT_DIR / 'scripts'}/",
                f"{host}:{remote_scripts}/",
            ])

        log("остановка сервисов, запущенных вручную через скрипты")
        ssh(
            host,
            f"cd {shlex.quote(remote)} && "
            "(python3 scripts/stop_services.py --only backend || true) && "
            "(python3 scripts/stop_services.py --only frontend || true)",
        )

        log("установка и запуск systemd-сервиса backend")
        run_sudo(
            host,
            [
                f"install -m 0644 {shlex.quote(remote_deploy + '/sophygo-api.service')} "
                "/etc/systemd/system/sophygo-api.service",
                "systemctl daemon-reload",
                "systemctl enable sophygo-api",
                "systemctl restart sophygo-api",
            ],
            password,
        )

        log("обновление конфига nginx")
        run_sudo(
            host,
            [
                f"install -m 0644 {shlex.quote(remote_deploy + '/nginx-sophygo.conf')} "
                "/etc/nginx/sites-available/sophygo",
                "ln -sfn /etc/nginx/sites-available/sophygo /etc/nginx/sites-enabled/sophygo",
                "nginx -t",
                "systemctl reload nginx",
            ],
            password,
        )

        log("проверка")
        ssh(
            host,
            "curl -sS -m 5 http://127.0.0.1:8080/healthz; echo; "
            "curl -sS -o /dev/null -w 'index: %{http_code}\\n' "
            "-H 'Host: sophygo.pro' http://127.0.0.1/; "
            "echo -n 'sophygo-api: '; systemctl is-active sophygo-api; "
            "echo -n 'enabled: '; systemctl is-enabled sophygo-api; "
            "ss -ltnp | grep ':5173' || echo '5173: free (Vite не запущен)'",
        )

    except subprocess.CalledProcessError as error:
        fail(f"команда завершилась с кодом {error.returncode}: {error.cmd}")

    log("готово: https://sophygo.pro")
    return 0


if __name__ == "__main__":
    sys.exit(main())
