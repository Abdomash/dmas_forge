from __future__ import annotations

import argparse
from pathlib import Path

from benchmark.benchlib import aggregate_results, load_result_rows, write_summary_csv, write_summary_md


def main() -> int:
    parser = argparse.ArgumentParser(description="Summarize benchmark result files")
    parser.add_argument("results", type=Path)
    parser.add_argument("--out", type=Path)
    args = parser.parse_args()

    out = args.out or args.results
    out.mkdir(parents=True, exist_ok=True)
    rows = aggregate_results(load_result_rows(args.results))
    write_summary_csv(out / "summary.csv", rows)
    write_summary_md(out / "summary.md", rows)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
