#!/usr/bin/env python3
"""Plot actual k6 JSON points; requires matplotlib, no cluster access."""
import argparse
from collections import defaultdict
from datetime import datetime, timezone, timedelta
import gzip
import json
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt


def timestamp(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp()


def percentile(values, p):
    values = sorted(values)
    position = (len(values) - 1) * p
    lower = int(position)
    upper = min(lower + 1, len(values) - 1)
    return values[lower] + (values[upper] - values[lower]) * (position - lower)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("dataset", type=Path)
    parser.add_argument("--events", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    durations, failures = [], []
    requests = dropped = 0
    with gzip.open(args.dataset, "rt") as stream:
        for line in stream:
            point = json.loads(line)
            if point.get("type") != "Point":
                continue
            metric, data = point["metric"], point["data"]
            if metric == "http_req_duration":
                durations.append((timestamp(data["time"]), data["value"]))
            elif metric == "http_req_failed":
                failures.append((timestamp(data["time"]), data["value"]))
            elif metric == "http_reqs":
                requests += data["value"]
            elif metric == "dropped_iterations":
                dropped += data["value"]
    if not durations or len(durations) != len(failures) or requests != len(durations):
        raise SystemExit("Incomplete or inconsistent k6 dataset")

    start = min(t for t, _ in durations)
    duration = max(t for t, _ in durations) - start
    windows = defaultdict(list)
    errors = defaultdict(list)
    for t, value in durations:
        windows[int((t - start) // 10)].append(value)
    for t, value in failures:
        errors[int((t - start) // 10)].append(value)
    indices = sorted(windows)
    x = [i * 10 + 5 for i in indices]
    window_p95 = [percentile(windows[i], 0.95) for i in indices]
    values = [v for _, v in durations]
    events = json.loads(args.events.read_text())
    args.output.mkdir(parents=True, exist_ok=True)
    plt.rcParams.update({"font.size": 11, "axes.spines.top": False, "axes.spines.right": False})

    def decorate(ax):
        for index, event in enumerate(events):
            offset = timestamp(event["time"]) - start
            if 0 <= offset <= duration:
                ax.axvline(offset, color=["#64748b", "#a855f7"][index % 2],
                           linestyle=":", label=event["label"])
        ax.set_xlim(0, 300)
        ax.set_xlabel("Время от начала прогона, с")
        ax.grid(alpha=0.2)
        ax.legend(loc="upper right", fontsize=9)

    fig, ax = plt.subplots(figsize=(12, 5), layout="constrained")
    ax.scatter([t-start for t, _ in durations], values, s=5, alpha=0.2,
               color="#2563eb", label="HTTP-запросы")
    ax.plot(x, window_p95, color="#0f766e", linewidth=2, label="p95 за 10 секунд")
    ax.axhline(300, color="#dc2626", linestyle="--", label="Порог p95: 300 мс")
    ax.set_ylabel("Задержка HTTP, мс")
    ax.set_title(f"Заказы во время eviction: общий p95 = {percentile(values, .95):.2f} мс")
    decorate(ax)
    fig.savefig(args.output / "34-memory-latency.png", dpi=170)
    plt.close(fig)

    fig, ax = plt.subplots(figsize=(12, 4.5), layout="constrained")
    ax.plot(x, [100 * sum(errors[i]) / len(errors[i]) for i in indices],
            color="#0f766e", marker="o", markersize=3, label="Ошибки за 10 секунд")
    ax.axhline(1, color="#dc2626", linestyle="--", label="Порог ошибок: 1%")
    ax.set_ylim(-0.1, 1.6)
    ax.set_ylabel("HTTP-ошибки, %")
    ax.set_title(f"{int(requests)} запросов; ошибок {int(sum(v for _, v in failures))}; "
                 f"пропущенных итераций {int(dropped)}")
    decorate(ax)
    fig.savefig(args.output / "35-memory-errors.png", dpi=170)
    plt.close(fig)

    print(json.dumps({
        "requests": int(requests), "errors": int(sum(v for _, v in failures)),
        "dropped_iterations": int(dropped), "p95_ms": percentile(values, .95),
        "mean_ms": sum(values)/len(values), "max_ms": max(values),
        "max_10s_p95_ms": max(window_p95),
        "first_point_msk": datetime.fromtimestamp(start, timezone(timedelta(hours=3))).isoformat(),
        "last_point_msk": datetime.fromtimestamp(start+duration, timezone(timedelta(hours=3))).isoformat(),
    }, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
