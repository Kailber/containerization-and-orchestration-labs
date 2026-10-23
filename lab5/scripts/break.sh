#!/bin/bash
set -e

VALUES="helm/shop/values.yaml"

case $((RANDOM % 3)) in
  0)
    sed -i '' 's/port: 5432/port: 5433/' "$VALUES"
    ;;
  1)
    sed -i '' 's/port: 53/port: 5353/' "$VALUES"
    ;;
  2)
    sed -i '' 's/shop-postgres/wrong-postgres/' "$VALUES"
    ;;
esac

helm upgrade shop ./helm/shop -n shop

echo "Поломка внесена"