# Benchmark

This directory contains a focused benchmark for comparing the same DMAS Forge examples across different wiring modes.

The goal is to measure the cost of distribution: how the same example behaves when it runs as a single container versus a more distributed setup such as HTTP, MCP, or A2A.

## What It Does

For each run in `benchmark/manifests/focused.json`, the benchmark:

- builds the example deployment
- starts the generated Docker setup
- waits until the frontend endpoint is ready
- sends requests from `benchmark/requests/*.jsonl`
- records startup metadata, per-request results, and resource samples
- generates summary tables with latency percentiles

The current manifest covers:

- `weather`: `single`, `docker`, `mcp`, `a2a`
- `marketing-agency`: `single`, `http`, `mcp`, `a2a`
- `travel-planning`: `single`, `docker`
- `financial-analyzer`: `single`, `http`, `mcp`, `a2a`

To keep runs repeatable, the benchmark uses mock search and image providers where available. It still uses your configured LLM backend for the agents themselves.

## What It Measures

Each run writes:

- `run.json`: the manifest entry for that run
- `startup.json`: readiness status, chosen endpoint, attempts, and the last HTTP or transport failure seen during startup
- `raw/requests-repeat-*.jsonl`: one row per request with status, success/failure, latency, response size, and request id
- `raw/resources.jsonl`: sampled resource usage for the containers started by that benchmark run when Compose can resolve them

The summary step writes:

- `summary.csv`
- `summary.md`

These summaries report:

- request count
- success count
- error count
- p50 latency
- p95 latency
- p99 latency

## How To Run It

1. Configure the shared benchmark model file:

- `benchmark/example_model.json`

2. Make sure Docker and Docker Compose are available.

3. If you want to run `financial-analyzer`, start the MCP server expected by the manifest at `http://localhost:8080`.

4. Run the benchmark from the repo root:

```bash
python3 -m benchmark.runner benchmark/manifests/focused.json
```

Optional flags:

```bash
python3 -m benchmark.runner benchmark/manifests/focused.json --run-id local-test
python3 -m benchmark.runner benchmark/manifests/focused.json --no-build
python3 -m benchmark.runner benchmark/manifests/focused.json --no-start
python3 -m benchmark.runner benchmark/manifests/focused.json --no-teardown
```

- `--run-id`: choose the output directory name under `benchmark/results/`
- `--no-build`: reuse an existing generated deployment
- `--no-start`: skip `docker compose up` and hit an already running deployment
- `--no-teardown`: leave the deployment running after the benchmark finishes

Results are written to:

```text
benchmark/results/<run-id>/
```

5. Summarize one run directory:

```bash
python3 -m benchmark.summary benchmark/results/<run-id>
```

## How To Review The Results

Start with:

- `benchmark/results/<run-id>/summary.md`

Then inspect individual run folders when needed:

- `run.json`: what was requested
- `startup.json`: whether startup succeeded, how long it took, and whether failures were HTTP errors or transport failures
- `raw/requests-repeat-*.jsonl`: request-level latency and failures
- `raw/resources.jsonl`: container-level CPU and memory samples during the run

A simple review flow is:

- compare the same example across modes
- compare the same profile across modes
- check `startup.json` before interpreting latency numbers
- inspect raw request rows when a mode has tail latency or failures

## Notes

- `provider_mode: "mock"` only mocks search and image side effects. It does not mock the LLM.
- Run `benchmark.summary` on a single `benchmark/results/<run-id>/` directory at a time. Summarizing `benchmark/results/` will combine multiple runs.
- If Compose cannot resolve the current run's container IDs, resource sampling falls back to the manifest target as-is.
