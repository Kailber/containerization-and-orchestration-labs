#!/bin/bash

NS=shop
API=$(kubectl get pod -n $NS -l app=api -o jsonpath='{.items[0].metadata.name}')
WORKER=$(kubectl get pod -n $NS -l app=worker -o jsonpath='{.items[0].metadata.name}')

test_pod() {
    POD=$1
    COMMAND=$2

    kubectl debug -n $NS pod/$POD \
        --image=nicolaka/netshoot \
        -- sh -c "$COMMAND" >/dev/null 2>&1

    DEBUG=$(kubectl get pod -n $NS $POD \
        -o jsonpath='{.spec.ephemeralContainers[-1].name}')

    sleep 3
    kubectl logs -n $NS $POD -c $DEBUG
}

echo "1. Client -> API (ALLOW)"
kubectl exec -n $NS test-client -- curl -sS --max-time 5 http://shop-api:8080/health

echo "2. Client -> PostgreSQL (DENY)"
kubectl exec -n $NS test-client -- curl -v --connect-timeout 3 --max-time 5 telnet://shop-postgres-rw:5432

echo "3. API -> PostgreSQL (ALLOW)"
test_pod "$API" "nc -zvw3 shop-postgres-rw 5432"

echo "4. Worker -> PostgreSQL (ALLOW)"
test_pod "$WORKER" "nc -zvw3 shop-postgres-rw 5432"

echo "5. API -> Worker (DENY)"
test_pod "$API" "nc -zvw3 $(kubectl get pod -n $NS $WORKER -o jsonpath='{.status.podIP}') 8082"

echo "6. Worker -> API (DENY)"
test_pod "$WORKER" "nc -zvw3 $(kubectl get pod -n $NS $API -o jsonpath='{.status.podIP}') 8080"

echo "7. API -> DNS (ALLOW)"
test_pod "$API" "nslookup shop-postgres-rw.shop.svc.cluster.local"

echo "8. Worker -> DNS (ALLOW)"
test_pod "$WORKER" "nslookup shop-postgres-rw.shop.svc.cluster.local"