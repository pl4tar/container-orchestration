#!/bin/bash
# Скрипт непрерывно убивает случайный под api или worker каждые 3 секунды
while true; do
  POD=$(kubectl get pods -n shop --no-headers -o custom-columns=":metadata.name" | grep -E "^(shop-api|shop-worker)" | shuf -n 1)
  if [ -n "$POD" ]; then
    echo "[$(date +%T)] Killing $POD"
    kubectl delete pod -n shop "$POD" --grace-period=0 --force >/dev/null 2>&1
  fi
  sleep 3
done