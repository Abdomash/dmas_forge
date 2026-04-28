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

To keep runs repeatable, the benchmark can use mock search and image providers where available. It still uses your configured LLM backend for the agents themselves. The `financial-analyzer` example can also target a lightweight mock MCP server provided under `benchmark/mock_mcp_server`.

## What It Measures

Each run writes:

- `run.json`: the manifest entry for that run
- `startup.json`: readiness status, chosen endpoint, attempts, and the last HTTP or transport failure seen during startup
- `raw/requests-repeat-*.jsonl`: one row per request with status, success/failure, latency, response size, and request id
- `raw/resources.jsonl`: sampled resource usage for the containers started by that benchmark run when Compose can resolve them
- `raw/jaeger/metadata.json`: Jaeger snapshot metadata for the benchmark time window
- `raw/jaeger/services.json`: raw Jaeger `/api/services` response captured before teardown
- `raw/jaeger/traces-*.json`: raw Jaeger `/api/traces` responses captured per discovered service before teardown
- `token_usage.json`: token totals collected from Jaeger spans for LLM calls made during the benchmark window

The summary step writes:

- `summary.csv`
- `summary.md`

These summaries report:

- request count
- success count
- error count
- LLM span count
- token-usage span count
- input tokens
- output tokens
- total tokens
- p50 latency
- p95 latency
- p99 latency

## How To Run It

1. Configure the shared benchmark model file:

- `benchmark/example_model.json`

2. Make sure Docker and Docker Compose are available.

- Your configured model endpoint in `benchmark/example_model.json` must be reachable from Docker containers, not just from the host machine.
- If the model is only reachable from the host, benchmark startup may succeed but the actual example requests can hang or fail inside the containers.

3. Choose whether each run uses mock or real side-effect providers in `benchmark/manifests/focused.json`:

- `"provider_mode": "mock"` enables mock search and image providers for that run.
- `"provider_mode": "real"` disables mocking and uses the example's real providers instead.
- The benchmark passes this through as `DMAS_SEARCH_API_MODE` and `DMAS_IMAGE_API_MODE` during `docker compose` startup.
- The checked-in focused manifest currently uses `"mock"` for every run.

4. Configure the MCP server list used by `financial-analyzer` builds:

- Edit `benchmark/financial_analyzer_mcp_servers.txt`.
- Put one MCP server URL per line.
- The checked-in default is `http://localhost:8080`.
- If a `financial-analyzer` manifest entry already includes `-mcp-servers=...` in `build_args`, that per-run value still wins.

5. If you do not want to use a real MCP server, start the benchmark's mock server:

```bash
go run ./benchmark/mock_mcp_server
```

- By default it listens on `http://localhost:8080`, which matches `benchmark/financial_analyzer_mcp_servers.txt`.
- It exposes deterministic `search_web` and `fetch_url` tools with canned data for Apple, Microsoft, and NVIDIA.
- To listen elsewhere, run `go run ./benchmark/mock_mcp_server --listen localhost:8081` and update `benchmark/financial_analyzer_mcp_servers.txt` to match.

6. Run the benchmark from the repo root:

```bash
python3 -m benchmark.runner benchmark/manifests/focused.json
```

Optional flags:

```bash
python3 -m benchmark.runner benchmark/manifests/focused.json --run-id local-test
python3 -m benchmark.runner benchmark/manifests/focused.json --example weather
python3 -m benchmark.runner benchmark/manifests/focused.json --example weather,travel-planning
python3 -m benchmark.runner benchmark/manifests/focused.json --example weather --example marketing-agency
python3 -m benchmark.runner benchmark/manifests/focused.json --no-build
python3 -m benchmark.runner benchmark/manifests/focused.json --no-start
python3 -m benchmark.runner benchmark/manifests/focused.json --no-teardown
```

- `--run-id`: choose the output directory name under `benchmark/results/`
- `--example`: only run selected examples from the manifest; repeat the flag or pass a comma-separated list
- `--no-build`: reuse an existing generated deployment
- `--no-start`: skip `docker compose up` and hit an already running deployment
- `--no-teardown`: leave the deployment running after the benchmark finishes

Results are written to:

```text
benchmark/results/<run-id>/
```

7. Summarize one run directory:

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
- `raw/jaeger/`: raw Jaeger API payloads saved before teardown so trace data survives after containers stop
- `token_usage.json`: aggregated token usage from Jaeger spans for that run

A simple review flow is:

- compare the same example across modes
- compare the same profile across modes
- check `startup.json` before interpreting latency numbers
- inspect raw request rows when a mode has tail latency or failures

## Notes

- `provider_mode: "mock"` only mocks search and image side effects. It does not mock the LLM.
- `provider_mode: "real"` is the non-mocked mode supported by the current benchmarked examples.
- The mock MCP server is optional and only affects `financial-analyzer` runs that point `-mcp-servers` at it.
- Token usage depends on the LLM provider returning usage in the OpenTelemetry spans. If the provider omits usage, `token_usage.json` will still be written but its token totals may be zero.
- The saved `raw/jaeger/traces-*.json` files are raw Jaeger API responses, not a standalone Jaeger UI database export.
- Run `benchmark.summary` on a single `benchmark/results/<run-id>/` directory at a time. Summarizing `benchmark/results/` will combine multiple runs.
- If Compose cannot resolve the current run's container IDs, resource sampling falls back to the manifest target as-is.
