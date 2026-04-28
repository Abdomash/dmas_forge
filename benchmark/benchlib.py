from __future__ import annotations

import csv
import hashlib
import json
import os
import re
import statistics
import subprocess
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]
ADDRESS_RE = re.compile(r"^(?P<host>[^:]+):(?P<port>\d+)$")
SERVICE_RE = re.compile(r"^  (?P<name>[A-Za-z0-9_.-]+):\s*$")
PORT_RE = re.compile(r'^      - ".*:(?P<internal>\d+)"\s*$')
JAEGER_TRACE_LIMIT = 5000


@dataclass(frozen=True)
class Endpoint:
    method: str
    url: str
    params: dict[str, str]


@dataclass(frozen=True)
class BenchmarkRun:
    example: str
    mode: str
    profile: str
    endpoint: Endpoint
    requests: Path
    build_args: list[str]
    provider_mode: str
    repeats: int
    duration: float
    concurrency: int
    timeout: float
    resource_targets: list[str]


class RequestCycler:
    def __init__(self, requests: list[dict[str, Any]]) -> None:
        if not requests:
            raise ValueError("at least one request is required")
        self._requests = requests
        self._lock = threading.Lock()
        self._idx = 0

    def next(self) -> dict[str, Any]:
        with self._lock:
            req = self._requests[self._idx % len(self._requests)]
            self._idx += 1
            return req


@dataclass(frozen=True)
class StartupMetadata:
    status: str
    original_manifest_url: str
    resolved_runtime_url: str
    discovery_source: str
    readiness_request_id: str
    attempts: int
    consecutive_successes_required: int
    wait_seconds: float
    candidate_urls: list[str]
    last_error: str = ""
    last_status: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {
            "status": self.status,
            "original_manifest_url": self.original_manifest_url,
            "resolved_runtime_url": self.resolved_runtime_url,
            "discovery_source": self.discovery_source,
            "readiness_request_id": self.readiness_request_id,
            "attempts": self.attempts,
            "consecutive_successes_required": self.consecutive_successes_required,
            "wait_seconds": self.wait_seconds,
            "candidate_urls": self.candidate_urls,
            "last_error": self.last_error,
            "last_status": self.last_status,
        }


def load_manifest(path: Path) -> list[BenchmarkRun]:
    with path.open("r", encoding="utf-8") as f:
        payload = json.load(f)
    entries = payload.get("runs", [payload])
    base_dir = path.parent
    return [parse_run(entry, base_dir) for entry in entries]


def parse_run(entry: dict[str, Any], base_dir: Path) -> BenchmarkRun:
    required = [
        "example",
        "mode",
        "profile",
        "endpoint",
        "requests",
        "provider_mode",
        "repeats",
        "duration",
        "concurrency",
        "timeout",
        "resource_targets",
    ]
    missing = [key for key in required if key not in entry]
    if missing:
        raise ValueError(f"missing manifest field(s): {', '.join(missing)}")
    endpoint = entry["endpoint"]
    if endpoint.get("method", "GET").upper() != "GET":
        raise ValueError("only GET endpoints are currently supported")
    requests_path = Path(entry["requests"])
    if not requests_path.is_absolute():
        requests_path = (base_dir / requests_path).resolve()
    return BenchmarkRun(
        example=str(entry["example"]),
        mode=str(entry["mode"]),
        profile=str(entry["profile"]),
        endpoint=Endpoint(
            method=endpoint.get("method", "GET").upper(),
            url=str(endpoint["url"]),
            params={str(k): str(v) for k, v in endpoint.get("params", {}).items()},
        ),
        requests=requests_path,
        build_args=[str(v) for v in entry.get("build_args", [])],
        provider_mode=str(entry["provider_mode"]),
        repeats=int(entry["repeats"]),
        duration=float(entry["duration"]),
        concurrency=int(entry["concurrency"]),
        timeout=float(entry["timeout"]),
        resource_targets=[str(v) for v in entry["resource_targets"]],
    )


def load_requests(path: Path) -> list[dict[str, Any]]:
    requests: list[dict[str, Any]] = []
    with path.open("r", encoding="utf-8") as f:
        for line_no, line in enumerate(f, start=1):
            stripped = line.strip()
            if not stripped:
                continue
            req = json.loads(stripped)
            if "id" not in req:
                raise ValueError(f"{path}:{line_no}: request is missing id")
            requests.append(req)
    if not requests:
        raise ValueError(f"{path}: no requests found")
    return requests


def build_url(endpoint: Endpoint, request: dict[str, Any]) -> str:
    params = dict(endpoint.params)
    params.update({str(k): str(v) for k, v in request.get("params", {}).items()})
    if "body" in request:
        params["req"] = json.dumps(request["body"], separators=(",", ":"))
    encoded = urllib.parse.urlencode(params)
    sep = "&" if "?" in endpoint.url else "?"
    return endpoint.url + (sep + encoded if encoded else "")


def resolve_endpoint_and_wait(
    run: BenchmarkRun,
    requests: list[dict[str, Any]],
    build_dir: Path,
    startup_timeout: float = 90.0,
    poll_interval: float = 0.5,
    consecutive_successes: int = 2,
) -> tuple[Endpoint | None, dict[str, Any]]:
    warmup_request = requests[0]
    candidates = discover_candidate_endpoints(run.endpoint, build_dir)
    candidate_urls = [endpoint.url for _, endpoint in candidates]
    consecutive: dict[str, int] = {endpoint.url: 0 for _, endpoint in candidates}
    attempts = 0
    started = time.monotonic()
    deadline = started + startup_timeout
    last_error = ""
    last_status = 0

    while time.monotonic() < deadline:
        for source, endpoint in candidates:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            attempts += 1
            probe_url = build_readiness_probe_url(endpoint)
            status, error, _, _ = perform_request(
                probe_url, max(1.0, min(run.timeout, 5.0, remaining))
            )
            last_error = error
            last_status = status
            if status > 0:
                consecutive[endpoint.url] = consecutive.get(endpoint.url, 0) + 1
                if consecutive[endpoint.url] >= consecutive_successes:
                    metadata = StartupMetadata(
                        status="ready",
                        original_manifest_url=run.endpoint.url,
                        resolved_runtime_url=endpoint.url,
                        discovery_source=source,
                        readiness_request_id=str(warmup_request["id"]),
                        attempts=attempts,
                        consecutive_successes_required=consecutive_successes,
                        wait_seconds=round(time.monotonic() - started, 3),
                        candidate_urls=candidate_urls,
                        last_error="",
                        last_status=last_status,
                    )
                    return endpoint, metadata.to_dict()
            else:
                consecutive[endpoint.url] = 0
        time.sleep(poll_interval)

    metadata = StartupMetadata(
        status="timeout",
        original_manifest_url=run.endpoint.url,
        resolved_runtime_url="",
        discovery_source="",
        readiness_request_id=str(warmup_request["id"]),
        attempts=attempts,
        consecutive_successes_required=consecutive_successes,
        wait_seconds=round(time.monotonic() - started, 3),
        candidate_urls=candidate_urls,
        last_error=last_error,
        last_status=last_status,
    )
    return None, metadata.to_dict()


def discover_candidate_endpoints(
    endpoint: Endpoint, build_dir: Path
) -> list[tuple[str, Endpoint]]:
    candidates: list[tuple[str, Endpoint]] = []
    seen: set[str] = set()

    for source, port in discover_ports_from_local_env(build_dir):
        candidate = endpoint_with_port(endpoint, port)
        if candidate.url in seen:
            continue
        seen.add(candidate.url)
        candidates.append((source, candidate))

    if not candidates:
        for source, port in discover_ports_from_compose(endpoint, build_dir):
            candidate = endpoint_with_port(endpoint, port)
            if candidate.url in seen:
                continue
            seen.add(candidate.url)
            candidates.append((source, candidate))

    if not candidates and endpoint.url not in seen:
        candidates.append(("manifest", endpoint))
    return candidates


def discover_ports_from_local_env(build_dir: Path) -> list[tuple[str, int]]:
    ports: list[tuple[str, int]] = []
    for key, value in load_local_env(build_dir).items():
        if not key.endswith("_HTTP_BIND_ADDR"):
            continue
        parsed = parse_address_port(value)
        if parsed is None:
            continue
        ports.append((f"local_env:{key}", parsed))
    if not ports:
        return []
    return [max(ports, key=lambda item: item[1])]


def build_readiness_probe_url(endpoint: Endpoint) -> str:
    parsed = urllib.parse.urlsplit(endpoint.url)
    return urllib.parse.urlunsplit(
        (parsed.scheme or "http", parsed.netloc, "/_benchmark_ready", "", "")
    )


def load_local_env(build_dir: Path) -> dict[str, str]:
    env_file = build_dir / ".local.env"
    if not env_file.exists():
        return {}

    values: dict[str, str] = {}
    for line in env_file.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in stripped:
            continue
        key, value = stripped.split("=", 1)
        values[key.strip()] = value.strip()
    return values


def discover_ports_from_compose(
    endpoint: Endpoint, build_dir: Path
) -> list[tuple[str, int]]:
    compose_file = build_dir / "docker" / "docker-compose.yml"
    if not compose_file.exists():
        return []

    compose_dir = compose_file.parent
    ports: list[tuple[str, int]] = []
    for service, internal_port in parse_compose_service_ports(compose_file):
        try:
            proc = subprocess.run(
                compose_cmd(build_dir) + ["port", service, str(internal_port)],
                cwd=compose_dir,
                check=False,
                capture_output=True,
                text=True,
            )
        except FileNotFoundError:
            return ports
        if proc.returncode != 0:
            continue
        for line in proc.stdout.splitlines():
            parsed = parse_address_port(line.strip())
            if parsed is None:
                continue
            ports.append((f"compose:{service}:{internal_port}", parsed))
    return ports


def parse_compose_service_ports(compose_file: Path) -> list[tuple[str, int]]:
    pairs: list[tuple[str, int]] = []
    service = ""
    in_ports = False
    for line in compose_file.read_text(encoding="utf-8").splitlines():
        service_match = SERVICE_RE.match(line)
        if service_match:
            service = service_match.group("name")
            in_ports = False
            continue
        if service and line == "    ports:":
            in_ports = True
            continue
        if in_ports:
            port_match = PORT_RE.match(line)
            if port_match:
                pairs.append((service, int(port_match.group("internal"))))
                continue
            if not line.startswith("      "):
                in_ports = False
    return pairs


def parse_address_port(value: str) -> int | None:
    candidate = value.strip()
    if candidate.startswith("[") and "]:" in candidate:
        candidate = candidate.split("]:", 1)[1]
    if ":" not in candidate:
        return None
    match = ADDRESS_RE.match(candidate)
    if match is not None:
        return int(match.group("port"))
    try:
        return int(candidate.rsplit(":", 1)[1])
    except ValueError:
        return None


def endpoint_with_port(endpoint: Endpoint, port: int) -> Endpoint:
    parsed = urllib.parse.urlsplit(endpoint.url)
    scheme = parsed.scheme or "http"
    netloc = f"localhost:{port}"
    url = urllib.parse.urlunsplit(
        (scheme, netloc, parsed.path, parsed.query, parsed.fragment)
    )
    return Endpoint(method=endpoint.method, url=url, params=dict(endpoint.params))


def provider_env(provider_mode: str) -> dict[str, str]:
    mode = provider_mode.strip().lower() or "mock"
    return {
        "DMAS_IMAGE_API_MODE": mode,
        "DMAS_SEARCH_API_MODE": mode,
    }


def example_wiring_dir(example: str) -> Path:
    path = REPO_ROOT / "examples" / example / "wiring"
    if not path.exists():
        raise ValueError(f"unknown example wiring dir: {path}")
    return path


def shared_model_file() -> Path:
    path = REPO_ROOT / "benchmark" / "example_model.json"
    if not path.exists():
        raise ValueError(f"missing benchmark model file: {path}")
    return path


def shared_financial_analyzer_mcp_servers_arg(build_args: list[str]) -> str | None:
    if any(arg.startswith("-mcp-servers=") for arg in build_args):
        return None

    path = REPO_ROOT / "benchmark" / "financial_analyzer_mcp_servers.txt"
    if not path.exists():
        return None

    servers = [
        line.strip()
        for line in path.read_text(encoding="utf-8").splitlines()
        if line.strip() and not line.lstrip().startswith("#")
    ]
    if not servers:
        return None
    return f"-mcp-servers={','.join(servers)}"


def build_deployment(run: BenchmarkRun, output_dir: Path) -> None:
    wiring_dir = example_wiring_dir(run.example)
    output_dir = output_dir.resolve()
    build_args = list(run.build_args)
    if run.example == "financial-analyzer":
        mcp_servers_arg = shared_financial_analyzer_mcp_servers_arg(build_args)
        if mcp_servers_arg is not None:
            build_args.append(mcp_servers_arg)
    cmd = [
        "go",
        "run",
        "main.go",
        "-w",
        run.mode,
        "-o",
        str(output_dir),
        f"-modfile={shared_model_file()}",
    ] + build_args
    subprocess.run(cmd, cwd=wiring_dir, check=True)
    normalize_generated_go_modules(output_dir)


def normalize_generated_go_modules(output_dir: Path) -> None:
    desired_versions = {
        "go.opentelemetry.io/otel": "v1.21.0",
        "go.opentelemetry.io/otel/metric": "v1.21.0",
        "go.opentelemetry.io/otel/trace": "v1.21.0",
        "go.opentelemetry.io/otel/sdk": "v1.21.0",
        "go.opentelemetry.io/otel/sdk/metric": "v1.21.0",
    }

    for go_mod in output_dir.rglob("go.mod"):
        contents = go_mod.read_text(encoding="utf-8")
        if "module blueprint/goproc/" not in contents:
            continue

        updated = contents
        for module, version in desired_versions.items():
            updated = re.sub(
                rf"^(?P<indent>\s*){re.escape(module)}\s+v[^\s]+(?P<suffix>\s*//.*)?$",
                rf"\g<indent>{module} {version}\g<suffix>",
                updated,
                flags=re.MULTILINE,
            )

        if updated == contents:
            continue

        go_mod.write_text(updated, encoding="utf-8")
        subprocess.run(["go", "mod", "tidy"], cwd=go_mod.parent, check=True)


def compose_up(build_dir: Path, env: dict[str, str]) -> None:
    build_dir = build_dir.resolve()
    compose_dir = build_dir / "docker"
    compose_env = os.environ.copy()
    compose_env.update(env)
    cmd = compose_cmd(build_dir)
    subprocess.run(cmd + ["build"], cwd=compose_dir, env=compose_env, check=True)
    subprocess.run(cmd + ["up", "-d"], cwd=compose_dir, env=compose_env, check=True)


def compose_down(build_dir: Path) -> None:
    build_dir = build_dir.resolve()
    compose_dir = build_dir / "docker"
    if compose_dir.exists():
        cmd = compose_cmd(build_dir)
        subprocess.run(
            cmd + ["down", "--remove-orphans"],
            cwd=compose_dir,
            check=False,
        )


def resolve_resource_targets(targets: list[str], build_dir: Path) -> list[str]:
    resolved: list[str] = []
    for target in targets:
        if target != "docker":
            resolved.append(target)
            continue

        container_ids = discover_compose_container_ids(build_dir)
        if not container_ids:
            resolved.append(target)
            continue

        resolved.extend(f"container:{container_id}" for container_id in container_ids)
    return dedupe_preserving_order(resolved)


def discover_compose_container_ids(build_dir: Path) -> list[str]:
    build_dir = build_dir.resolve()
    compose_dir = build_dir / "docker"
    if not compose_dir.exists():
        return []

    cmd = compose_cmd(build_dir)

    try:
        proc = subprocess.run(
            cmd + ["ps", "-q"],
            cwd=compose_dir,
            check=False,
            capture_output=True,
            text=True,
        )
    except FileNotFoundError:
        return []

    if proc.returncode != 0:
        return []
    return [line.strip() for line in proc.stdout.splitlines() if line.strip()]


def compose_cmd(build_dir: Path) -> list[str]:
    build_dir = build_dir.resolve()
    env_file = build_dir / ".local.env"
    cmd = ["docker", "compose", "-p", compose_project_name(build_dir)]
    if env_file.exists():
        cmd += ["--env-file", str(env_file)]
    return cmd


def compose_project_name(build_dir: Path) -> str:
    build_dir = build_dir.resolve()
    run_name = sanitize_compose_project_part(build_dir.parent.name)
    run_id = sanitize_compose_project_part(build_dir.parent.parent.name)
    digest = hashlib.sha1(str(build_dir).encode("utf-8")).hexdigest()[:8]
    project = f"benchmark-{run_id}-{run_name}-{digest}"
    return project[:63].rstrip("-")


def sanitize_compose_project_part(value: str) -> str:
    cleaned = re.sub(r"[^a-z0-9_-]+", "-", value.lower()).strip("-")
    return cleaned or "run"


def dedupe_preserving_order(values: list[str]) -> list[str]:
    seen: set[str] = set()
    deduped: list[str] = []
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        deduped.append(value)
    return deduped


def start_resource_sampler(
    run: BenchmarkRun, out_path: Path, interval: float = 1.0
) -> tuple[threading.Event, threading.Thread]:
    stop = threading.Event()

    def sample_loop() -> None:
        out_path.parent.mkdir(parents=True, exist_ok=True)
        with out_path.open("w", encoding="utf-8") as f:
            while not stop.is_set():
                ts = time.time()
                for row in collect_resource_samples(run.resource_targets):
                    row["timestamp"] = ts
                    f.write(json.dumps(row, sort_keys=True) + "\n")
                f.flush()
                stop.wait(interval)

    thread = threading.Thread(target=sample_loop, daemon=True)
    thread.start()
    return stop, thread


def collect_resource_samples(targets: list[str]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for target in targets:
        if target == "docker":
            rows.extend(sample_docker_stats([]))
        elif target.startswith("container:"):
            rows.extend(sample_docker_stats([target.split(":", 1)[1]]))
        elif target.startswith("process:"):
            rows.extend(sample_process(target.split(":", 1)[1]))
    return rows


def sample_docker_stats(containers: list[str]) -> list[dict[str, Any]]:
    cmd = ["docker", "stats", "--no-stream", "--format", "{{json .}}"] + containers
    try:
        proc = subprocess.run(cmd, check=False, capture_output=True, text=True)
    except FileNotFoundError:
        return []
    rows: list[dict[str, Any]] = []
    for line in proc.stdout.splitlines():
        try:
            payload = json.loads(line)
        except json.JSONDecodeError:
            continue
        rows.append(
            {
                "kind": "container",
                "name": payload.get("Name", ""),
                "id": payload.get("Container", ""),
                "cpu_percent": payload.get("CPUPerc", ""),
                "memory_usage": payload.get("MemUsage", ""),
                "memory_percent": payload.get("MemPerc", ""),
            }
        )
    return rows


def sample_process(pattern: str) -> list[dict[str, Any]]:
    cmd = ["ps", "-eo", "pid,pcpu,rss,comm,args"]
    try:
        proc = subprocess.run(cmd, check=False, capture_output=True, text=True)
    except FileNotFoundError:
        return []
    rows: list[dict[str, Any]] = []
    for line in proc.stdout.splitlines()[1:]:
        parts = line.split(None, 4)
        if len(parts) < 5:
            continue
        pid, cpu, rss, comm, args = parts
        if pattern not in args:
            continue
        rows.append(
            {
                "kind": "process",
                "name": comm,
                "id": pid,
                "cpu_percent": cpu,
                "memory_rss_kib": rss,
            }
        )
    return rows


def collect_jaeger_snapshot(
    build_dir: Path, start_time_s: float, end_time_s: float
) -> dict[str, Any]:
    jaeger_base_url = discover_jaeger_base_url(build_dir)
    snapshot: dict[str, Any] = {
        "status": "unavailable",
        "jaeger_base_url": jaeger_base_url or "",
        "window_start_unix_seconds": round(start_time_s, 3),
        "window_end_unix_seconds": round(end_time_s, 3),
        "services_payload": {},
        "traces_by_service": {},
        "last_error": "",
    }
    if jaeger_base_url is None:
        snapshot["last_error"] = "missing JAEGER_UI_BIND_ADDR"
        return snapshot

    start_us = int(max(0.0, start_time_s - 2.0) * 1_000_000)
    end_us = int((end_time_s + 2.0) * 1_000_000)

    for attempt in range(3):
        try:
            services_payload = fetch_json(f"{jaeger_base_url}/api/services")
            services = [
                str(service) for service in (services_payload.get("data") or [])
            ]
            traces_by_service: dict[str, dict[str, Any]] = {}
            traces_found = 0
            for service in services:
                traces_payload = fetch_jaeger_traces(
                    jaeger_base_url, service, start_us, end_us
                )
                traces_by_service[service] = traces_payload
                traces_found += len(traces_payload.get("data") or [])

            snapshot["services_payload"] = services_payload
            snapshot["traces_by_service"] = traces_by_service
            if traces_found > 0 or attempt == 2:
                snapshot["status"] = "ok"
                return snapshot
        except Exception as exc:  # noqa: BLE001 - benchmark records collection failures.
            snapshot["last_error"] = str(exc)
            if attempt == 2:
                return snapshot
        time.sleep(0.5)

    return snapshot


def collect_token_usage(
    build_dir: Path, start_time_s: float, end_time_s: float
) -> dict[str, Any]:
    snapshot = collect_jaeger_snapshot(build_dir, start_time_s, end_time_s)
    return token_usage_from_jaeger_snapshot(snapshot)


def discover_jaeger_base_url(build_dir: Path) -> str | None:
    addr = load_local_env(build_dir).get("JAEGER_UI_BIND_ADDR", "")
    port = parse_address_port(addr)
    if port is None:
        return None
    return f"http://localhost:{port}"


def fetch_json(url: str, timeout: float = 5.0) -> dict[str, Any]:
    with urllib.request.urlopen(url, timeout=timeout) as resp:
        return json.loads(resp.read().decode("utf-8"))


def fetch_jaeger_traces(
    jaeger_base_url: str, service: str, start_us: int, end_us: int
) -> dict[str, Any]:
    params = urllib.parse.urlencode(
        {
            "service": service,
            "start": str(start_us),
            "end": str(end_us),
            "limit": str(JAEGER_TRACE_LIMIT),
        }
    )
    return fetch_json(f"{jaeger_base_url}/api/traces?{params}")


def token_usage_from_jaeger_snapshot(snapshot: dict[str, Any]) -> dict[str, Any]:
    summary: dict[str, Any] = {
        "status": snapshot.get("status", "unavailable"),
        "jaeger_base_url": snapshot.get("jaeger_base_url", ""),
        "window_start_unix_seconds": snapshot.get("window_start_unix_seconds", 0),
        "window_end_unix_seconds": snapshot.get("window_end_unix_seconds", 0),
        "services_queried": [
            str(service)
            for service in (snapshot.get("services_payload", {}).get("data") or [])
        ],
        "traces_seen": 0,
        "llm_call_spans": 0,
        "token_usage_available_spans": 0,
        "input_tokens": 0,
        "output_tokens": 0,
        "total_tokens": 0,
        "last_error": snapshot.get("last_error", ""),
    }
    seen_traces: set[str] = set()
    seen_spans: set[tuple[str, str]] = set()

    for traces_payload in snapshot.get("traces_by_service", {}).values():
        for trace in traces_payload.get("data") or []:
            trace_id = str(trace.get("traceID", ""))
            if trace_id:
                seen_traces.add(trace_id)
            for span in trace.get("spans") or []:
                span_id = str(span.get("spanID", ""))
                span_key = (trace_id, span_id)
                if span_key in seen_spans:
                    continue
                seen_spans.add(span_key)

                tags = span_tags(span)
                if not has_llm_token_tags(tags):
                    continue

                summary["llm_call_spans"] += 1
                if parse_bool(tags.get("llm.token_usage_available")):
                    summary["token_usage_available_spans"] += 1
                summary["input_tokens"] += parse_int(tags.get("llm.input_tokens"))
                summary["output_tokens"] += parse_int(tags.get("llm.output_tokens"))
                summary["total_tokens"] += parse_int(tags.get("llm.total_tokens"))

    summary["traces_seen"] = len(seen_traces)
    return summary


def write_jaeger_snapshot(path: Path, snapshot: dict[str, Any]) -> None:
    path.mkdir(parents=True, exist_ok=True)
    metadata = {
        "status": snapshot.get("status", "unavailable"),
        "jaeger_base_url": snapshot.get("jaeger_base_url", ""),
        "window_start_unix_seconds": snapshot.get("window_start_unix_seconds", 0),
        "window_end_unix_seconds": snapshot.get("window_end_unix_seconds", 0),
        "last_error": snapshot.get("last_error", ""),
        "services": [
            str(service)
            for service in (snapshot.get("services_payload", {}).get("data") or [])
        ],
    }
    write_json(path / "metadata.json", metadata)
    write_json(path / "services.json", snapshot.get("services_payload", {}))
    for service, traces_payload in sorted(
        snapshot.get("traces_by_service", {}).items()
    ):
        write_json(path / f"traces-{sanitize_filename(service)}.json", traces_payload)


def sanitize_filename(value: str) -> str:
    cleaned = re.sub(r"[^A-Za-z0-9_.-]+", "-", value).strip("-.")
    return cleaned or "value"


def span_tags(span: dict[str, Any]) -> dict[str, Any]:
    tags: dict[str, Any] = {}
    for tag in span.get("tags") or []:
        key = tag.get("key")
        if not key:
            continue
        tags[str(key)] = tag.get("value")
    return tags


def has_llm_token_tags(tags: dict[str, Any]) -> bool:
    return any(
        key in tags
        for key in (
            "llm.token_usage_available",
            "llm.input_tokens",
            "llm.output_tokens",
            "llm.total_tokens",
        )
    )


def parse_bool(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value.strip().lower() == "true"
    return False


def parse_int(value: Any) -> int:
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, (int, float)):
        return int(value)
    if isinstance(value, str):
        try:
            return int(value.strip())
        except ValueError:
            return 0
    return 0


def run_profile(
    run: BenchmarkRun, requests: list[dict[str, Any]], result_dir: Path
) -> list[dict[str, Any]]:
    result_dir.mkdir(parents=True, exist_ok=True)
    all_results: list[dict[str, Any]] = []
    for repeat in range(run.repeats):
        cycler = RequestCycler(requests)
        repeat_results = execute_repeat(run, cycler, repeat)
        all_results.extend(repeat_results)
        write_jsonl(result_dir / f"requests-repeat-{repeat + 1}.jsonl", repeat_results)
    return all_results


def execute_repeat(
    run: BenchmarkRun, cycler: RequestCycler, repeat: int
) -> list[dict[str, Any]]:
    if run.profile == "smoke":
        total = max(1, run.concurrency)
        return execute_fixed_count(run, cycler, repeat, total)
    return execute_duration(run, cycler, repeat)


def execute_fixed_count(
    run: BenchmarkRun, cycler: RequestCycler, repeat: int, count: int
) -> list[dict[str, Any]]:
    with ThreadPoolExecutor(max_workers=run.concurrency) as executor:
        futures = [
            executor.submit(send_one, run, cycler.next(), repeat, i)
            for i in range(count)
        ]
        return [future.result() for future in as_completed(futures)]


def execute_duration(
    run: BenchmarkRun, cycler: RequestCycler, repeat: int
) -> list[dict[str, Any]]:
    stop_at = time.monotonic() + run.duration
    results: list[dict[str, Any]] = []
    sequence = 0
    with ThreadPoolExecutor(max_workers=run.concurrency) as executor:
        futures = set()
        while time.monotonic() < stop_at or futures:
            while time.monotonic() < stop_at and len(futures) < run.concurrency:
                futures.add(
                    executor.submit(send_one, run, cycler.next(), repeat, sequence)
                )
                sequence += 1
            done = {future for future in futures if future.done()}
            if not done:
                time.sleep(0.01)
                continue
            for future in done:
                results.append(future.result())
            futures -= done
    return results


def send_one(
    run: BenchmarkRun, request: dict[str, Any], repeat: int, sequence: int
) -> dict[str, Any]:
    url = build_url(run.endpoint, request)
    status, error, size, latency_ms = perform_request(url, run.timeout)
    return {
        "example": run.example,
        "mode": run.mode,
        "profile": run.profile,
        "repeat": repeat + 1,
        "sequence": sequence,
        "request_id": request["id"],
        "status": status,
        "ok": 200 <= status < 300 and error == "",
        "latency_ms": latency_ms,
        "response_bytes": size,
        "error": error,
        "trace_id": "",
        "provider_mode": run.provider_mode,
    }


def perform_request(url: str, timeout: float) -> tuple[int, str, int, float]:
    started = time.perf_counter()
    status = 0
    error = ""
    size = 0
    try:
        with urllib.request.urlopen(url, timeout=timeout) as resp:
            body = resp.read()
            status = resp.status
            size = len(body)
    except urllib.error.HTTPError as exc:
        status = exc.code
        error = str(exc)
        try:
            size = len(exc.read())
        except Exception:
            size = 0
    except Exception as exc:  # noqa: BLE001 - benchmark records all failures.
        error = str(exc)
    latency_ms = (time.perf_counter() - started) * 1000.0
    return status, error, size, latency_ms


def write_jsonl(path: Path, rows: list[dict[str, Any]]) -> None:
    with path.open("w", encoding="utf-8") as f:
        for row in rows:
            f.write(json.dumps(row, sort_keys=True) + "\n")


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.write_text(json.dumps(payload, indent=2, sort_keys=True), encoding="utf-8")


def load_result_rows(result_dir: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for path in sorted(result_dir.glob("**/requests-repeat-*.jsonl")):
        with path.open("r", encoding="utf-8") as f:
            for line in f:
                stripped = line.strip()
                if stripped:
                    rows.append(json.loads(stripped))
    return rows


def load_token_usage_rows(result_dir: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for path in sorted(result_dir.glob("**/token_usage.json")):
        run_path = path.parent / "run.json"
        if not run_path.exists():
            continue
        run = json.loads(run_path.read_text(encoding="utf-8"))
        token_usage = json.loads(path.read_text(encoding="utf-8"))
        rows.append(
            {
                "example": run["example"],
                "mode": run["mode"],
                "profile": run["profile"],
                "provider_mode": run.get("provider_mode", ""),
                "llm_call_spans": int(token_usage.get("llm_call_spans", 0)),
                "token_usage_available_spans": int(
                    token_usage.get("token_usage_available_spans", 0)
                ),
                "input_tokens": int(token_usage.get("input_tokens", 0)),
                "output_tokens": int(token_usage.get("output_tokens", 0)),
                "total_tokens": int(token_usage.get("total_tokens", 0)),
            }
        )
    return rows


def aggregate_results(
    rows: list[dict[str, Any]], token_rows: list[dict[str, Any]] | None = None
) -> list[dict[str, Any]]:
    groups: dict[tuple[str, str, str, str], list[dict[str, Any]]] = {}
    for row in rows:
        key = (
            row["example"],
            row["mode"],
            row["profile"],
            row.get("provider_mode", ""),
        )
        groups.setdefault(key, []).append(row)

    token_groups: dict[tuple[str, str, str, str], dict[str, int]] = {}
    for row in token_rows or []:
        key = (
            row["example"],
            row["mode"],
            row["profile"],
            row.get("provider_mode", ""),
        )
        token_group = token_groups.setdefault(
            key,
            {
                "llm_call_spans": 0,
                "token_usage_available_spans": 0,
                "input_tokens": 0,
                "output_tokens": 0,
                "total_tokens": 0,
            },
        )
        for field in token_group:
            token_group[field] += int(row.get(field, 0))

    summaries: list[dict[str, Any]] = []
    for (example, mode, profile, provider_mode), group in sorted(groups.items()):
        latencies = sorted(float(row["latency_ms"]) for row in group if row.get("ok"))
        token_group = token_groups.get(
            (example, mode, profile, provider_mode),
            {
                "llm_call_spans": 0,
                "token_usage_available_spans": 0,
                "input_tokens": 0,
                "output_tokens": 0,
                "total_tokens": 0,
            },
        )
        summaries.append(
            {
                "example": example,
                "mode": mode,
                "profile": profile,
                "provider_mode": provider_mode,
                "requests": len(group),
                "successes": sum(1 for row in group if row.get("ok")),
                "errors": sum(1 for row in group if not row.get("ok")),
                "p50_ms": percentile(latencies, 50),
                "p95_ms": percentile(latencies, 95),
                "p99_ms": percentile(latencies, 99),
                **token_group,
            }
        )
    return summaries


def percentile(values: list[float], pct: int) -> float:
    if not values:
        return 0.0
    if len(values) == 1:
        return values[0]
    idx = (len(values) - 1) * (pct / 100.0)
    lo = int(idx)
    hi = min(lo + 1, len(values) - 1)
    if lo == hi:
        return values[lo]
    weight = idx - lo
    return values[lo] * (1 - weight) + values[hi] * weight


def write_summary_csv(path: Path, rows: list[dict[str, Any]]) -> None:
    fieldnames = [
        "example",
        "mode",
        "profile",
        "provider_mode",
        "requests",
        "successes",
        "errors",
        "llm_call_spans",
        "token_usage_available_spans",
        "input_tokens",
        "output_tokens",
        "total_tokens",
        "p50_ms",
        "p95_ms",
        "p99_ms",
    ]
    with path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)


def write_summary_md(path: Path, rows: list[dict[str, Any]]) -> None:
    lines = [
        "# Benchmark Summary",
        "",
        "| Example | Mode | Profile | Provider | Requests | Successes | Errors | LLM spans | Token spans | Input tokens | Output tokens | Total tokens | p50 ms | p95 ms | p99 ms |",
        "|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|",
    ]
    for row in rows:
        lines.append(
            "| {example} | {mode} | {profile} | {provider_mode} | {requests} | {successes} | {errors} | {llm_call_spans} | {token_usage_available_spans} | {input_tokens} | {output_tokens} | {total_tokens} | {p50_ms:.2f} | {p95_ms:.2f} | {p99_ms:.2f} |".format(
                **row
            )
        )
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def median_repeat_value(values: list[float]) -> float:
    return float(statistics.median(values)) if values else 0.0
