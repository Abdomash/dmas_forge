# Benchmark Summary

| Example | Mode | Profile | Provider | Requests | Successes | Errors | LLM spans | Token spans | Input tokens | Output tokens | Total tokens | p50 ms | p95 ms | p99 ms |
|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| weather | a2a | burst | mock | 15 | 15 | 0 | 0 | 0 | 0 | 0 | 0 | 29770.27 | 36333.67 | 36393.85 |
| weather | docker | steady | mock | 9 | 9 | 0 | 0 | 0 | 0 | 0 | 0 | 10877.43 | 13883.76 | 14424.98 |
| weather | mcp | steady | mock | 8 | 8 | 0 | 0 | 0 | 0 | 0 | 0 | 13061.74 | 16383.31 | 17479.07 |
| weather | single | smoke | mock | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 10657.55 | 10657.55 | 10657.55 |
