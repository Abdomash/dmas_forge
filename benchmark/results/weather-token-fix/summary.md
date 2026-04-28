# Benchmark Summary

| Example | Mode | Profile | Provider | Requests | Successes | Errors | LLM spans | Token spans | Input tokens | Output tokens | Total tokens | p50 ms | p95 ms | p99 ms |
|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| weather | a2a | burst | mock | 15 | 15 | 0 | 28 | 28 | 11305 | 9977 | 21282 | 31088.41 | 42905.35 | 43544.33 |
| weather | docker | steady | mock | 8 | 8 | 0 | 14 | 14 | 5811 | 4516 | 10327 | 11951.35 | 16757.48 | 18515.37 |
| weather | mcp | steady | mock | 9 | 9 | 0 | 17 | 17 | 6938 | 5591 | 12529 | 12001.83 | 15241.11 | 15891.55 |
| weather | single | smoke | mock | 1 | 1 | 0 | 2 | 2 | 881 | 718 | 1599 | 13434.24 | 13434.24 | 13434.24 |
