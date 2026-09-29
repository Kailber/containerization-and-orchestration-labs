#!/bin/bash

CG=/sys/fs/cgroup/monitoring
OUT="${1:-metrics.csv}"

echo "timestamp,mem_mb,mem_max_mb,cpu_ms,nr_throttled" > "$OUT"

START=$(date +%s)

while true; do
  NOW=$(date +%s)
  ELAPSED=$((NOW - START))

  MEM=$(cat $CG/memory.current 2>/dev/null || echo 0)
  MEM_MAX=$(cat $CG/memory.max 2>/dev/null || echo 0)
  CPU_US=$(awk '/^usage_usec/ {print $2}' $CG/cpu.stat 2>/dev/null || echo 0)
  THROTTLED=$(awk '/^nr_throttled/ {print $2}' $CG/cpu.stat 2>/dev/null || echo 0)
  MEM_MB=$((MEM / 1024 / 1024))
  MEM_MAX_MB=$((MEM_MAX / 1024 / 1024))
  CPU_MS=$((CPU_US / 1000))

  echo "$ELAPSED,$MEM_MB,$MEM_MAX_MB,$CPU_MS,$THROTTLED" >> "$OUT"

  if [ $ELAPSED -ge 60 ]; then
    break
  fi

  sleep 1
done
