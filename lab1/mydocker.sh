#!/bin/bash

set -e

echo "Starting API in isolated namespaces..."

exec unshare \
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