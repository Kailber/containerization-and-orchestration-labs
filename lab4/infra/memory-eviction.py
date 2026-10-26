#!/usr/bin/env python3
"""Configure a reversible kubelet eviction threshold on the kind worker."""
import argparse
import json
from pathlib import Path
import re
import subprocess

NODE = "lab4-worker"
CONFIG = "/var/lib/kubelet/config.yaml"
BACKUP = CONFIG + ".lab4-memory-backup"
MIB = 1024 * 1024


def docker(*args, **kwargs):
    return subprocess.run(["docker", *args], check=True, text=True, **kwargs)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--restore", action="store_true")
    args = parser.parse_args()
    if args.restore:
        docker("exec", NODE, "cp", BACKUP, CONFIG)
        docker("exec", NODE, "systemctl", "restart", "kubelet")
        print("Original kubelet configuration restored; kubelet restarted.")
        return

    host = dict(line.split(":", 1) for line in Path("/proc/meminfo").read_text().splitlines())
    host_available = int(host["MemAvailable"].split()[0]) * 1024
    if host_available < 3 * 1024**3:
        raise SystemExit("Less than 3 GiB available on the host: stop before adding memory load.")

    result = docker(
        "exec", "lab4-control-plane", "kubectl",
        "--kubeconfig=/etc/kubernetes/admin.conf", "get", "--raw",
        f"/api/v1/nodes/{NODE}/proxy/stats/summary", capture_output=True,
    )
    memory = json.loads(result.stdout)["node"]["memory"]
    available_mib = memory["availableBytes"] // MIB
    threshold_mib = available_mib - 512
    if threshold_mib <= 0:
        raise SystemExit("Insufficient node memory for the experiment.")

    original = docker("exec", NODE, "cat", CONFIG, capture_output=True).stdout
    block = re.search(r"(?m)^evictionHard:\n(?:[ \t]+[^\n]*\n)*", original)
    if block is None:
        raise SystemExit("Unexpected kubelet configuration: no evictionHard mapping.")
    if re.search(r"(?m)^\s+memory\.available:", block.group()):
        raise SystemExit("A memory threshold already exists; inspect or restore it first.")
    updated = original[:block.end()] + f'  memory.available: "{threshold_mib}Mi"\n' + original[block.end():]
    updated, count = re.subn(
        r"(?m)^evictionPressureTransitionPeriod:.*$",
        "evictionPressureTransitionPeriod: 2m0s", updated,
    )
    if count != 1:
        raise SystemExit("Unexpected evictionPressureTransitionPeriod setting.")

    # Never overwrite the original backup, including after a restored run.
    docker("exec", NODE, "sh", "-c", f"test -f {BACKUP} || cp {CONFIG} {BACKUP}")
    docker("exec", "-i", NODE, "sh", "-c", f"cat > {CONFIG}.lab4-new", input=updated)
    docker("exec", NODE, "mv", CONFIG + ".lab4-new", CONFIG)
    docker("exec", NODE, "systemctl", "restart", "kubelet")
    print(f"Host MemAvailable: {host_available // MIB} MiB")
    print(f"Worker memory.available before setup: {available_mib} MiB")
    print(f"evictionHard memory.available: {threshold_mib}Mi (512 MiB headroom)")
    print("MemoryPressure transition period: 2 minutes; no batch memory increase yet.")
    print("Restore: python3 ./infra/memory-eviction.py --restore")


if __name__ == "__main__":
    main()
