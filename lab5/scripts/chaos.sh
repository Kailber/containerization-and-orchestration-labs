#!/bin/bash

NAMESPACE="shop"
INTERVAL=15

echo "Да начнется хаос в $NAMESPACE"

while true; do
    POD=$(kubectl get pods -n "$NAMESPACE" \
        -l 'app in (api,worker)' \
        -o name | awk 'BEGIN {srand()} {pods[NR]=$0} END {if (NR) print pods[int(rand()*NR)+1]}')

    if [ -n "$POD" ]; then
        echo "Удален $POD"
        kubectl delete -n "$NAMESPACE" "$POD" --wait=false
    fi

    sleep "$INTERVAL"
done