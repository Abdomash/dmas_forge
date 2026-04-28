from __future__ import annotations

import argparse
import json
import time
from dataclasses import replace
from pathlib import Path

from benchmark.benchlib import (
    build_deployment,
    compose_down,
    compose_up,
    load_manifest,
    load_requests,
    provider_env,
    resolve_endpoint_and_wait,
    run_profile,
    start_resource_sampler,
)


def main() -> int:
    parser = argparse.ArgumentParser(description="Run DMAS Forge focused benchmarks")
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--results", type=Path, default=Path("benchmark/results"))
    parser.add_argument("--run-id", default=time.strftime("%Y%m%d-%H%M%S"))
    parser.add_argument("--no-build", action="store_true")
    parser.add_argument("--no-start", action="store_true")
    parser.add_argument("--no-teardown", action="store_true")
    args = parser.parse_args()

    runs = load_manifest(args.manifest)
    root = args.results / args.run_id
    root.mkdir(parents=True, exist_ok=True)

    for run in runs:
        name = f"{run.example}-{run.mode}-{run.profile}"
        run_dir = root / name
        build_dir = run_dir / "build"
        result_dir = run_dir / "raw"
        run_dir.mkdir(parents=True, exist_ok=True)
        build_dir.mkdir(parents=True, exist_ok=True)
        result_dir.mkdir(parents=True, exist_ok=True)
        env = provider_env(run.provider_mode)
        requests = load_requests(run.requests)
        (run_dir / "run.json").write_text(
            json.dumps(run.__dict__, default=str, indent=2, sort_keys=True),
            encoding="utf-8",
        )
        try:
            if not args.no_build:
                build_deployment(run, build_dir)
            if not args.no_start:
                compose_up(build_dir, env)

            resolved_endpoint, startup = resolve_endpoint_and_wait(
                run, requests, build_dir
            )
            (run_dir / "startup.json").write_text(
                json.dumps(startup, indent=2, sort_keys=True), encoding="utf-8"
            )
            if resolved_endpoint is None:
                raise RuntimeError(
                    f"startup readiness failed for {name}: {startup.get('last_error') or startup.get('status')}"
                )

            resolved_run = replace(run, endpoint=resolved_endpoint)
            stop, thread = start_resource_sampler(
                resolved_run, run_dir / "raw" / "resources.jsonl"
            )
            try:
                run_profile(resolved_run, requests, result_dir)
            finally:
                stop.set()
                thread.join(timeout=2)
        finally:
            if not args.no_teardown:
                compose_down(build_dir)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
