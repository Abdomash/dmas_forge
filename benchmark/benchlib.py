from __future__ import annotations

import csv
import json
import os
import statistics
import subprocess
import threading
import time
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]


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


def build_deployment(run: BenchmarkRun, output_dir: Path) -> None:
    wiring_dir = example_wiring_dir(run.example)
    cmd = [
        "go",
        "run",
        "main.go",
        "-w",
        run.mode,
        "-o",
        str(output_dir),
        "-modfile=./example_model.json",
    ] + run.build_args
    subprocess.run(cmd, cwd=wiring_dir, check=True)


def compose_up(build_dir: Path, env: dict[str, str]) -> None:
    compose_dir = build_dir / "docker"
    compose_env = os.environ.copy()
    compose_env.update(env)
    env_file = build_dir / ".local.env"
    cmd = ["docker", "compose"]
    if env_file.exists():
        cmd += ["--env-file", str(env_file)]
    subprocess.run(cmd + ["build"], cwd=compose_dir, env=compose_env, check=True)
    subprocess.run(cmd + ["up", "-d"], cwd=compose_dir, env=compose_env, check=True)


def compose_down(build_dir: Path) -> None:
    compose_dir = build_dir / "docker"
    if compose_dir.exists():
        subprocess.run(["docker", "compose", "down", "--remove-orphans"], cwd=compose_dir, check=False)


def start_resource_sampler(run: BenchmarkRun, out_path: Path, interval: float = 1.0) -> tuple[threading.Event, threading.Thread]:
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


def run_profile(run: BenchmarkRun, requests: list[dict[str, Any]], result_dir: Path) -> list[dict[str, Any]]:
    result_dir.mkdir(parents=True, exist_ok=True)
    all_results: list[dict[str, Any]] = []
    for repeat in range(run.repeats):
        cycler = RequestCycler(requests)
        repeat_results = execute_repeat(run, cycler, repeat)
        all_results.extend(repeat_results)
        write_jsonl(result_dir / f"requests-repeat-{repeat + 1}.jsonl", repeat_results)
    return all_results


def execute_repeat(run: BenchmarkRun, cycler: RequestCycler, repeat: int) -> list[dict[str, Any]]:
    if run.profile == "smoke":
        total = max(1, run.concurrency)
        return execute_fixed_count(run, cycler, repeat, total)
    return execute_duration(run, cycler, repeat)


def execute_fixed_count(run: BenchmarkRun, cycler: RequestCycler, repeat: int, count: int) -> list[dict[str, Any]]:
    with ThreadPoolExecutor(max_workers=run.concurrency) as executor:
        futures = [executor.submit(send_one, run, cycler.next(), repeat, i) for i in range(count)]
        return [future.result() for future in as_completed(futures)]


def execute_duration(run: BenchmarkRun, cycler: RequestCycler, repeat: int) -> list[dict[str, Any]]:
    stop_at = time.monotonic() + run.duration
    results: list[dict[str, Any]] = []
    sequence = 0
    with ThreadPoolExecutor(max_workers=run.concurrency) as executor:
        futures = set()
        while time.monotonic() < stop_at or futures:
            while time.monotonic() < stop_at and len(futures) < run.concurrency:
                futures.add(executor.submit(send_one, run, cycler.next(), repeat, sequence))
                sequence += 1
            done = {future for future in futures if future.done()}
            if not done:
                time.sleep(0.01)
                continue
            for future in done:
                results.append(future.result())
            futures -= done
    return results


def send_one(run: BenchmarkRun, request: dict[str, Any], repeat: int, sequence: int) -> dict[str, Any]:
    url = build_url(run.endpoint, request)
    started = time.perf_counter()
    status = 0
    error = ""
    size = 0
    try:
        with urllib.request.urlopen(url, timeout=run.timeout) as resp:
            body = resp.read()
            status = resp.status
            size = len(body)
    except Exception as exc:  # noqa: BLE001 - benchmark records all failures.
        error = str(exc)
    latency_ms = (time.perf_counter() - started) * 1000.0
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


def write_jsonl(path: Path, rows: list[dict[str, Any]]) -> None:
    with path.open("w", encoding="utf-8") as f:
        for row in rows:
            f.write(json.dumps(row, sort_keys=True) + "\n")


def load_result_rows(result_dir: Path) -> list[dict[str, Any]]:
	rows: list[dict[str, Any]] = []
	for path in sorted(result_dir.glob("**/requests-repeat-*.jsonl")):
		with path.open("r", encoding="utf-8") as f:
			for line in f:
				stripped = line.strip()
				if stripped:
					rows.append(json.loads(stripped))
	return rows


def aggregate_results(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    groups: dict[tuple[str, str, str, str], list[dict[str, Any]]] = {}
    for row in rows:
        key = (row["example"], row["mode"], row["profile"], row.get("provider_mode", ""))
        groups.setdefault(key, []).append(row)

    summaries: list[dict[str, Any]] = []
    for (example, mode, profile, provider_mode), group in sorted(groups.items()):
        latencies = sorted(float(row["latency_ms"]) for row in group if row.get("ok"))
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
    fieldnames = ["example", "mode", "profile", "provider_mode", "requests", "successes", "errors", "p50_ms", "p95_ms", "p99_ms"]
    with path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)


def write_summary_md(path: Path, rows: list[dict[str, Any]]) -> None:
    lines = [
        "# Benchmark Summary",
        "",
        "| Example | Mode | Profile | Provider | Requests | Successes | Errors | p50 ms | p95 ms | p99 ms |",
        "|---|---|---|---|---:|---:|---:|---:|---:|---:|",
    ]
    for row in rows:
        lines.append(
            "| {example} | {mode} | {profile} | {provider_mode} | {requests} | {successes} | {errors} | {p50_ms:.2f} | {p95_ms:.2f} | {p99_ms:.2f} |".format(
                **row
            )
        )
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def median_repeat_value(values: list[float]) -> float:
    return float(statistics.median(values)) if values else 0.0
