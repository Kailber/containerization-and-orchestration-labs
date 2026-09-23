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
        exec ./api/api
    '