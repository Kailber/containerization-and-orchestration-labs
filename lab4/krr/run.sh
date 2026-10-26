#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
krr_strategy=${KRR_STRATEGY:-simple}
krr_report=${KRR_REPORT:-report.json}
krr_history=${KRR_HISTORY:-1}
krr_config=$(mktemp /tmp/lab4-krr-kubeconfig.XXXXXX)
trap 'rm -f -- "$krr_config"' EXIT

# Internal endpoint is reachable from the network namespace of the kind node.
# Keep the temporary credentials outside the repository and remove them on exit.
kind get kubeconfig --name lab4 --internal > "$krr_config"
prometheus_ip=$(kubectl --context kind-lab4 -n lab2 get service \
  monitoring-kube-prometheus-prometheus -o jsonpath='{.spec.clusterIP}')
test -n "$prometheus_ip"

docker run --rm \
  --network container:lab4-control-plane \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e NO_PROXY='*' -e no_proxy='*' \
  -e KRR_JSON_OUTPUT="/work/$krr_report" \
  -v "$krr_config:/kubeconfig:ro" \
  -v "$script_dir:/work" \
  --entrypoint python \
  us-central1-docker.pkg.dev/genuine-flight-317411/devel/krr:v1.30.0 \
  /work/cnpg.py "$krr_strategy" \
  -k /kubeconfig -c kind-lab4 -n lab4 \
  -p "http://${prometheus_ip}:9090" \
  --history_duration "$krr_history" --timeframe_duration 1 --points_required 10 \
  --width 180 "$@"
