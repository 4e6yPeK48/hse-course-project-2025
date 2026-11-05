from __future__ import annotations

import json
import os
import shutil
import socket
import subprocess
import time
from pathlib import Path
from http.cookies import SimpleCookie
from typing import Any, Dict, Iterable, Optional, Tuple
from urllib import error, request as urllib_request

import pytest

ROOT = Path(__file__).resolve().parents[1]
BACKEND_DIR = ROOT / "app" / "backend"


def _kill_process_tree(proc: subprocess.Popen) -> None:
    if proc.poll() is not None:
        return
    if os.name == "nt":
        subprocess.run(
            ["taskkill", "/PID", str(proc.pid), "/T", "/F"],
            capture_output=True,
            text=True,
            check=False,
        )
    else:
        proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=5)


@pytest.fixture(scope="session")
def backend_binary(tmp_path_factory: pytest.TempPathFactory) -> Path:
    go = shutil.which("go")
    if not go:
        pytest.fail("Go toolchain is not available in PATH")

    bin_dir = tmp_path_factory.mktemp("backend-bin")
    binary_name = "backend-test.exe" if os.name == "nt" else "backend-test"
    binary_path = bin_dir / binary_name

    build = subprocess.run(
        [go, "build", "-o", str(binary_path), "."],
        cwd=BACKEND_DIR,
        capture_output=True,
        text=True,
        check=False,
    )
    if build.returncode != 0:
        pytest.fail(
            "backend build failed\n"
            f"stdout:\n{build.stdout}\n"
            f"stderr:\n{build.stderr}"
        )

    return binary_path


class BackendProcess:
    def __init__(self, binary: Path, data_dir: Path) -> None:
        self.binary = binary
        self.data_dir = data_dir
        self.proc = None
        self.log_file = None
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            self.port = sock.getsockname()[1]
        self.base_url = f"http://127.0.0.1:{self.port}"

    def start(self) -> None:
        self.log_file = (self.data_dir / "server.log").open("a", encoding="utf-8")
        self.proc = subprocess.Popen(
            [str(self.binary)], cwd=self.data_dir,
            env={**os.environ, "PORT": str(self.port), "DATABASE_PATH": str(self.data_dir / "game.db")},
            stdout=self.log_file, stderr=subprocess.STDOUT, text=True,
        )
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline and self.proc.poll() is None:
            try:
                with urllib_request.urlopen(f"{self.base_url}/api/hello", timeout=1) as resp:
                    if resp.status == 200 and resp.read().decode().strip() == "hello from backend":
                        return
            except (OSError, error.URLError):
                time.sleep(0.05)
        self.stop()
        pytest.fail("backend did not start:\n" + (self.data_dir / "server.log").read_text())

    def stop(self) -> None:
        if self.proc is not None:
            _kill_process_tree(self.proc)
        if self.log_file is not None:
            self.log_file.close()


@pytest.fixture(scope="session")
def backend_server(backend_binary: Path, tmp_path_factory: pytest.TempPathFactory) -> Iterable[str]:
    server = BackendProcess(backend_binary, tmp_path_factory.mktemp("backend-data"))
    server.start()
    try:
        yield server.base_url
    finally:
        server.stop()


@pytest.fixture
def restartable_backend(backend_binary, tmp_path):
    server = BackendProcess(backend_binary, tmp_path)
    server.start()
    try:
        yield server
    finally:
        server.stop()


class _NoRedirectHandler(urllib_request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class APIClient:
    """A browser session per real actor; payload IDs only select already-issued cookies."""

    def __init__(self, base_url: str) -> None:
        self.base_url = base_url.rstrip("/")
        self.cookies: Dict[Tuple[str, str], str] = {}

    def actor_headers(self, code: str, player_id: str) -> Dict[str, str]:
        return {"Cookie": self.cookies[(code, player_id)]}

    def request(
        self, method: str, path: str, payload=None, *, headers=None,
        allow_error=False, actor_id=None, form_payload=None, no_redirect=False,
    ) -> Tuple[int, str, Dict[str, str]]:
        req_headers = {"Accept": "application/json"}
        room_parts = path.split("?", 1)[0].split("/")
        code = room_parts[3] if len(room_parts) > 3 and room_parts[1:3] == ["api", "game"] else None
        actor_id = actor_id or (payload or {}).get("player_id")
        if code and (code, actor_id) in self.cookies:
            req_headers.update(self.actor_headers(code, actor_id))
        data = None
        if form_payload is not None:
            req_headers.update({"Accept": "text/html", "Content-Type": "application/x-www-form-urlencoded"})
            data = form_payload.encode("utf-8")
        elif payload is not None:
            req_headers["Content-Type"] = "application/json"
            data = json.dumps(payload).encode("utf-8")
        req_headers.update(headers or {})
        req = urllib_request.Request(f"{self.base_url}{path}", data=data, headers=req_headers, method=method)
        opener = urllib_request.build_opener(_NoRedirectHandler) if no_redirect else urllib_request.build_opener()
        try:
            with opener.open(req, timeout=10) as resp:
                status, body, response_headers = resp.status, resp.read().decode("utf-8"), dict(resp.headers)
        except error.HTTPError as exc:
            status, body, response_headers = exc.code, exc.read().decode("utf-8"), dict(exc.headers)
            if not allow_error:
                raise AssertionError(f"{method} {path} -> {status}: {body}") from exc
        if status in (200, 201) and path in ("/api/create", "/api/join") and "Set-Cookie" in response_headers:
            identity = json.loads(body)
            cookie = SimpleCookie()
            cookie.load(response_headers["Set-Cookie"])
            actor = identity.get("facilitator_id") or identity.get("player_id")
            self.cookies[(identity["game_code"], actor)] = "; ".join(f"{name}={m.value}" for name, m in cookie.items())
        return status, body, response_headers

    def request_no_redirect(self, method, path, payload=None, *, form_payload=None, headers=None):
        return self.request(method, path, payload, form_payload=form_payload, headers=headers,
                            allow_error=True, no_redirect=True)

    def form(self, method, path, payload, *, headers=None, allow_error=False):
        return self.request(method, path, form_payload=payload, headers=headers, allow_error=allow_error)

    def json(self, method, path, payload=None, *, headers=None, allow_error=False, actor_id=None):
        status, body, response_headers = self.request(method, path, payload, headers=headers,
                                                     allow_error=allow_error, actor_id=actor_id)
        return status, json.loads(body) if body else None, response_headers

    def text(self, method, path, *, actor_id=None):
        return self.request(method, path, headers={"Accept": "text/plain"}, actor_id=actor_id)


@pytest.fixture()
def api_client(backend_server: str) -> APIClient:
    return APIClient(backend_server)


