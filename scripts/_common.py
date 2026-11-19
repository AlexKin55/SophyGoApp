"""Общие функции для скриптов сборки и управления сервисами SophyGo.

Только стандартная библиотека Python 3. Скрипты рассчитаны на Linux.
"""

from __future__ import annotations

import os
import re
import shutil
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import NoReturn

SCRIPTS_DIR = Path(__file__).resolve().parent
ROOT_DIR = SCRIPTS_DIR.parent
BACKEND_DIR = ROOT_DIR / "backend"
FRONTEND_DIR = ROOT_DIR / "frontend"

RUN_DIR = SCRIPTS_DIR / ".run"
BACKEND_BINARY = BACKEND_DIR / "bin" / "sophygo-api"

GO_BIN_DIR = Path("/usr/local/go/bin")
BACKEND_PORT = 8080
FRONTEND_PORT = 5173

DEFAULT_LOG_DIR = Path("/tmp/sophygoapp")

ENV_FILES = {
    "backend": [BACKEND_DIR / ".env"],
    "frontend": [
        FRONTEND_DIR / ".env",
        FRONTEND_DIR / ".env.local",
        FRONTEND_DIR / ".env.development.local",
    ],
}

SERVICE_FILES = {
    "backend": {
        "pid": RUN_DIR / "backend.pid",
        "port": BACKEND_PORT,
    },
    "frontend": {
        "pid": RUN_DIR / "frontend.pid",
        "port": FRONTEND_PORT,
    },
}

HEALTH_URLS = {
    "backend": f"http://127.0.0.1:{BACKEND_PORT}/healthz",
    "frontend": f"http://localhost:{FRONTEND_PORT}/",
}


def log(message: str) -> None:
    """Печатает сообщение с префиксом скрипта."""
    print(f"[sophygo] {message}", flush=True)


def fail(message: str, exit_code: int = 1) -> NoReturn:
    print(f"[sophygo] ОШИБКА: {message}", file=sys.stderr, flush=True)
    raise SystemExit(exit_code)


def build_env() -> dict[str, str]:
    """Окружение процесса с добавленным путём к Go."""
    env = os.environ.copy()
    path_parts = env.get("PATH", "").split(os.pathsep)
    if GO_BIN_DIR.is_dir() and str(GO_BIN_DIR) not in path_parts:
        env["PATH"] = os.pathsep.join([str(GO_BIN_DIR), *path_parts])
    env.setdefault("PYTHONUNBUFFERED", "1")
    return env


def require_tool(name: str) -> str:
    """Возвращает путь к исполняемому файлу или завершает скрипт."""
    path = shutil.which(name, path=build_env().get("PATH"))
    if path:
        return path
    fail(f"не найден {name}. Установите его или добавьте в PATH.")
    return name  # недостижимо, нужно для типизации


def ensure_run_dir() -> None:
    RUN_DIR.mkdir(parents=True, exist_ok=True)


def load_env_values(name: str) -> dict[str, str]:
    """Читает KEY=VALUE из .env-файлов сервиса (последний файл важнее)."""
    values: dict[str, str] = {}
    for path in ENV_FILES.get(name, []):
        try:
            content = path.read_text(encoding="utf-8")
        except OSError:
            continue
        for line in content.splitlines():
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            line = line.removeprefix("export ").strip()
            key, separator, value = line.partition("=")
            if not separator:
                continue
            key = key.strip()
            value = value.strip()
            if len(value) >= 2 and value[0] == value[-1] and value[0] in ("'", '"'):
                value = value[1:-1]
            if key:
                values[key] = value
    return values


def service_log_path(name: str) -> Path:
    """Путь к логу сервиса из .env; по умолчанию /tmp/sophygoapp/<name>.log."""
    values = load_env_values(name)
    log_dir = Path(values.get("LOG_DIR") or DEFAULT_LOG_DIR)
    raw = values.get("LOG_FILE") or ""
    if raw:
        path = Path(raw)
        return path if path.is_absolute() else log_dir / path
    return log_dir / f"{name}.log"


def read_pid(name: str) -> int | None:
    path = SERVICE_FILES[name]["pid"]
    try:
        raw = path.read_text(encoding="utf-8").strip()
    except FileNotFoundError:
        return None
    try:
        return int(raw)
    except ValueError:
        return None


def is_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def write_pid(name: str, pid: int) -> None:
    ensure_run_dir()
    SERVICE_FILES[name]["pid"].write_text(f"{pid}\n", encoding="utf-8")


def remove_pid(name: str) -> None:
    try:
        SERVICE_FILES[name]["pid"].unlink()
    except FileNotFoundError:
        pass


def pids_on_port(port: int) -> list[int]:
    """Возвращает PID процессов, слушающих порт (через ss)."""
    ss = shutil.which("ss")
    if not ss:
        return []
    try:
        result = subprocess.run(
            [ss, "-ltnp"],
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )
    except (OSError, subprocess.SubprocessError):
        return []

    pids: set[int] = set()
    for line in result.stdout.splitlines():
        if f":{port}" not in line:
            continue
        for match in re.finditer(r"pid=(\d+)", line):
            pids.add(int(match.group(1)))
    return sorted(pids)


def is_project_process(pid: int) -> bool:
    """Проверяет, что процесс относится к этому проекту, а не чужой."""
    project = str(ROOT_DIR)

    # Рабочий каталог — самый надёжный признак: backend запускается из
    # backend/, frontend из frontend/, даже если бинарник лежит в /tmp.
    try:
        cwd = os.readlink(f"/proc/{pid}/cwd")
        if cwd == project or cwd.startswith(project + os.sep):
            return True
    except (FileNotFoundError, PermissionError, OSError):
        pass

    try:
        cmdline = Path(f"/proc/{pid}/cmdline").read_bytes().replace(b"\0", b" ").decode(
            "utf-8", errors="replace"
        )
    except (FileNotFoundError, PermissionError, OSError):
        return False

    markers = (project, "vite", "cmd/server", "sophygo-api")
    return any(marker in cmdline for marker in markers)


def _children_map() -> dict[int, list[int]]:
    """Карта ppid -> список дочерних pid из /proc."""
    children: dict[int, list[int]] = {}
    try:
        entries = os.listdir("/proc")
    except OSError:
        return children

    for entry in entries:
        if not entry.isdigit():
            continue
        pid = int(entry)
        try:
            stat = Path(f"/proc/{pid}/stat").read_text(encoding="utf-8", errors="replace")
        except (FileNotFoundError, PermissionError, OSError):
            continue
        closing = stat.rfind(")")
        if closing < 0:
            continue
        fields = stat[closing + 1 :].split()
        if len(fields) < 2:
            continue
        try:
            ppid = int(fields[1])
        except ValueError:
            continue
        children.setdefault(ppid, []).append(pid)
    return children


def _descendants(pid: int) -> list[int]:
    """Все потомки процесса (без самого процесса)."""
    children = _children_map()
    result: list[int] = []
    stack = list(children.get(pid, []))
    while stack:
        current = stack.pop()
        result.append(current)
        stack.extend(children.get(current, []))
    return result


def stop_process_tree(pid: int, name: str, timeout: float = 10.0) -> bool:
    """Останавливает процесс и его потомков: SIGTERM, затем SIGKILL.

    Группу процессов убиваем только если процесс — её лидер (так запускаются
    сервисы из start_services.py). Для чужих групп это опасно: можно задеть
    терминал, поэтому там останавливаем только само дерево процесса.
    """
    if not is_alive(pid):
        remove_pid(name)
        return False

    log(f"останавливаю {name} (pid {pid})…")
    targets = _descendants(pid) + [pid]

    try:
        pgid = os.getpgid(pid)
    except ProcessLookupError:
        remove_pid(name)
        return False

    if pgid == pid:
        try:
            os.killpg(pgid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    else:
        for target in targets:
            try:
                os.kill(target, signal.SIGTERM)
            except ProcessLookupError:
                continue

    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if not is_alive(pid):
            remove_pid(name)
            log(f"{name} остановлен")
            return True
        time.sleep(0.2)

    log(f"{name} не завершился за {timeout:.0f} с, отправляю SIGKILL")
    if pgid == pid:
        try:
            os.killpg(pgid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    else:
        for target in targets:
            try:
                os.kill(target, signal.SIGKILL)
            except ProcessLookupError:
                continue
    time.sleep(0.3)
    remove_pid(name)
    log(f"{name} остановлен принудительно")
    return True


def stop_service(name: str, timeout: float = 10.0) -> bool:
    """Останавливает сервис по pid-файлу или по занятому порту."""
    stopped = False

    pid = read_pid(name)
    if pid and is_alive(pid):
        stopped = stop_process_tree(pid, name, timeout=timeout)
    else:
        remove_pid(name)

    port = SERVICE_FILES[name]["port"]
    for port_pid in pids_on_port(port):
        if is_project_process(port_pid):
            stopped = stop_process_tree(port_pid, name, timeout=timeout) or stopped

    if not stopped:
        log(f"{name} не запущен")
    return stopped


def wait_for_http(name: str, timeout: float = 30.0) -> bool:
    """Ждёт, пока сервис начнёт отвечать по HTTP."""
    url = HEALTH_URLS[name]
    deadline = time.monotonic() + timeout
    last_error = ""

    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=1.0) as response:
                if 200 <= response.status < 500:
                    return True
        except urllib.error.HTTPError as error:
            if error.code < 500:
                return True
            last_error = f"HTTP {error.code}"
        except (urllib.error.URLError, TimeoutError, OSError) as error:
            last_error = str(error)
        time.sleep(0.4)

    log(f"{name} не ответил за {timeout:.0f} с ({last_error})")
    return False


def start_service(
    name: str,
    command: list[str],
    cwd: Path,
    timeout: float = 30.0,
    wait: bool = True,
) -> int | None:
    """Запускает сервис в отдельной сессии и пишет pid и лог."""
    existing = read_pid(name)
    if existing and is_alive(existing):
        log(f"{name} уже запущен (pid {existing})")
        return existing

    ensure_run_dir()
    log_file = service_log_path(name)
    log_file.parent.mkdir(parents=True, exist_ok=True)
    log(f"запускаю {name}: {' '.join(command)}")
    log(f"лог: {log_file}")

    with log_file.open("ab") as stream:
        process = subprocess.Popen(
            command,
            cwd=str(cwd),
            env=build_env(),
            stdin=subprocess.DEVNULL,
            stdout=stream,
            stderr=subprocess.STDOUT,
            start_new_session=True,
            close_fds=True,
        )

    write_pid(name, process.pid)

    if wait:
        if wait_for_http(name, timeout):
            log(f"{name} готов: {HEALTH_URLS[name]}")
        else:
            log(f"{name} запущен (pid {process.pid}), но health-check не прошёл. Смотрите лог.")
            return process.pid

    return process.pid
