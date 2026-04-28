from __future__ import annotations

import argparse
import json
import time
from dataclasses import replace
from pathlib import Path

from benchmark.benchlib import (
    build_deployment,
    collect_jaeger_snapshot,
    compose_down,
    compose_up,
    load_manifest,
    load_requests,
    provider_env,
    resolve_resource_targets,
    resolve_endpoint_and_wait,
    run_profile,
    start_resource_sampler,
    token_usage_from_jaeger_snapshot,
    write_jaeger_snapshot,
)


def parse_example_filters(values: list[str]) -> set[str]:
    selected: set[str] = set()
    for value in values:
        for item in value.split(","):
            example = item.strip()
            if example:
                selected.add(example)
    return selected


def main() -> int:
    parser = argparse.ArgumentParser(description="Run DMAS Forge focused benchmarks")
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--results", type=Path, default=Path("benchmark/results"))
    parser.add_argument("--run-id", default=time.strftime("%Y%m%d-%H%M%S"))
    parser.add_argument(
        "--example",
        action="append",
        default=[],
        help="Only run selected example names; repeat the flag or use a comma-separated list",
    )
    parser.add_argument("--no-build", action="store_true")
    parser.add_argument("--no-start", action="store_true")
    parser.add_argument("--no-teardown", action="store_true")
    args = parser.parse_args()

    runs = load_manifest(args.manifest)
    selected_examples = parse_example_filters(args.example)
    if selected_examples:
        runs = [run for run in runs if run.example in selected_examples]
        if not runs:
            raise ValueError(
                "no manifest runs matched the selected example filter(s): "
                + ", ".join(sorted(selected_examples))
            )

    root = (args.results / args.run_id).resolve()
    root.mkdir(parents=True, exist_ok=True)

    for run in runs:
        name = f"{run.example}-{run.mode}-{run.profile}"
        run_dir = root / name
        build_dir = run_dir / "build"
        result_dir = run_dir / "raw"
        run_dir.mkdir(parents=True, exist_ok=True)
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

            resolved_run = replace(
                run,
                endpoint=resolved_endpoint,
                resource_targets=resolve_resource_targets(
                    run.resource_targets, build_dir
                ),
            )
            stop, thread = start_resource_sampler(
                resolved_run, run_dir / "raw" / "resources.jsonl"
            )
            try:
                benchmark_started = time.time()
                run_profile(resolved_run, requests, result_dir)
                benchmark_finished = time.time()
                jaeger_snapshot = collect_jaeger_snapshot(
                    build_dir, benchmark_started, benchmark_finished
                )
                write_jaeger_snapshot(run_dir / "raw" / "jaeger", jaeger_snapshot)
                (run_dir / "token_usage.json").write_text(
                    json.dumps(
                        token_usage_from_jaeger_snapshot(jaeger_snapshot),
                        indent=2,
                        sort_keys=True,
                    ),
                    encoding="utf-8",
                )
            finally:
                stop.set()
                thread.join(timeout=2)
        finally:
            if not args.no_teardown:
                compose_down(build_dir)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
