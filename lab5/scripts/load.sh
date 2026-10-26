#!/bin/bash

NAMESPACE="shop"
INTERVAL=0.2
URL="http://shop-api.shop.svc.cluster.local:8080/health"
LOG="load-result.csv"

echo "time,status" > "$LOG"

while true; do
    STATUS=$(kubectl exec -n "$NAMESPACE" test-client -- \
        curl -sS -o /dev/null -w "%{http_code}" --max-time 5 "$URL")

    [ -z "$STATUS" ] && STATUS="000"

    echo "$(date +%s),$STATUS" >> "$LOG"
    echo "$(date '+%H:%M:%S') HTTP $STATUS"

    sleep "$INTERVAL"
done