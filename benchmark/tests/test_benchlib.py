from __future__ import annotations

import json
import tempfile
import unittest
import urllib.parse
from pathlib import Path

from benchmark.benchlib import (
    Endpoint,
    RequestCycler,
    aggregate_results,
    build_url,
    load_manifest,
    load_requests,
    provider_env,
    write_jsonl,
)


class BenchmarkHarnessTests(unittest.TestCase):
    def test_manifest_parsing(self) -> None:
        manifest = Path("benchmark/manifests/focused.json")
        runs = load_manifest(manifest)
        self.assertGreaterEqual(len(runs), 12)
        weather_single = next(run for run in runs if run.example == "weather" and run.mode == "single")
        self.assertEqual(weather_single.profile, "smoke")
        self.assertTrue(weather_single.requests.name.endswith("weather.jsonl"))

    def test_request_loading_requires_ids(self) -> None:
        requests = load_requests(Path("benchmark/requests/weather.jsonl"))
        self.assertEqual(requests[0]["id"], "london")

    def test_endpoint_request_generation_for_query_params(self) -> None:
        endpoint = Endpoint(method="GET", url="http://localhost:12345/Query", params={})
        url = build_url(endpoint, {"id": "x", "params": {"query": "London Weather"}})
        self.assertEqual(url, "http://localhost:12345/Query?query=London+Weather")

    def test_endpoint_request_generation_for_body_payload(self) -> None:
        endpoint = Endpoint(method="GET", url="http://localhost:12345/CreateCampaign", params={})
        url = build_url(endpoint, {"id": "x", "body": {"brand_name": "A", "keywords": ["a"]}})
        self.assertIn("/CreateCampaign?req=", url)
        decoded = json.loads(urllib.parse.parse_qs(urllib.parse.urlparse(url).query)["req"][0])
        self.assertEqual(decoded["brand_name"], "A")

    def test_round_robin_query_cycling(self) -> None:
        cycler = RequestCycler([{"id": "a"}, {"id": "b"}])
        self.assertEqual([cycler.next()["id"] for _ in range(5)], ["a", "b", "a", "b", "a"])

    def test_provider_mode_env(self) -> None:
        env = provider_env("mock")
        self.assertEqual(env["DMAS_IMAGE_API_MODE"], "mock")
        self.assertEqual(env["DMAS_SEARCH_API_MODE"], "mock")

    def test_summary_aggregation_from_sample_result_files(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            result_dir = Path(tmp)
            rows = [
                {"example": "weather", "mode": "single", "profile": "smoke", "provider_mode": "mock", "ok": True, "latency_ms": 10},
                {"example": "weather", "mode": "single", "profile": "smoke", "provider_mode": "mock", "ok": True, "latency_ms": 30},
                {"example": "weather", "mode": "single", "profile": "smoke", "provider_mode": "mock", "ok": False, "latency_ms": 99},
            ]
            write_jsonl(result_dir / "requests-repeat-1.jsonl", rows)
            loaded = []
            for line in (result_dir / "requests-repeat-1.jsonl").read_text(encoding="utf-8").splitlines():
                loaded.append(json.loads(line))
            summary = aggregate_results(loaded)
            self.assertEqual(summary[0]["requests"], 3)
            self.assertEqual(summary[0]["successes"], 2)
            self.assertEqual(summary[0]["errors"], 1)
            self.assertEqual(summary[0]["p50_ms"], 20)


if __name__ == "__main__":
    unittest.main()
