import sys
import csv
import matplotlib.pyplot as plt


def load(csv_path):
    data = {"t": [], "mem": [], "mem_max": [], "cpu": [], "throttled": [], "pids": []}
    with open(csv_path) as f:
        reader = csv.DictReader(f)
        for row in reader:
            data["t"].append(int(row["timestamp"]))
            data["mem"].append(int(row["mem_mb"]))
            data["mem_max"].append(int(row["mem_max_mb"]))
            data["cpu"].append(int(row["cpu_ms"]))
            data["throttled"].append(int(row["nr_throttled"]))
            data["pids"].append(int(row["pids"]))
    return data


def plot(data, out_path):
    fig, axes = plt.subplots(3, 1, figsize=(12, 10), sharex=True)
    fig.suptitle("Метрики", fontsize=14)

    axes[0].plot(data["t"], data["mem"], "b-", label="memory.current (MB)")
    axes[0].plot(data["t"], data["mem_max"], "r--", label="memory.max (MB)")
    axes[0].set_ylabel("Память (MB)")
    axes[0].legend()
    axes[0].grid(True)

    axes[1].plot(data["t"], data["cpu"], "g-", label="cpu usage (ms)")
    axes[1].set_ylabel("CPU (ms)")
    axes[1].legend()
    axes[1].grid(True)

    axes[2].plot(data["t"], data["throttled"], "m-", label="nr_throttled")
    axes[2].set_xlabel("Время (сек)")
    axes[2].set_ylabel("Throttled")
    axes[2].legend()
    axes[2].grid(True)

    plt.tight_layout()
    plt.savefig(out_path, dpi=100)


if __name__ == "__main__":
    csv_path = sys.argv[1] if len(sys.argv) > 1 else "metrics.csv"
    out_path = sys.argv[2] if len(sys.argv) > 2 else "dashboard.png"
    plot(load(csv_path), out_path)