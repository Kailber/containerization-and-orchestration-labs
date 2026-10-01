#!/bin/bash

set -e

echo "Starting API in isolated namespaces..."

systemd-run \
--user \
--scope \
--collect \
--unit=lab1-memory \
-p MemoryMax=128M \
-p MemorySwapMax=0 \
-p CPUQuota=50% \
-p TasksMax=20 \
-p TimeoutStopSec=5s \
    unshare \
    --pid \
    --mount \
    --net \
    --uts \
    --ipc \
    --user \
    --map-root-user \
    --fork \
    sh -c '
        mount -t proc proc /proc
        hostname isolated-api
        exec setpriv --bounding-set=-all --inh-caps=-all --ambient-caps=-all --no-new-privs --seccomp-filter=seccomp/seccomp.bpf ./api/api
    '