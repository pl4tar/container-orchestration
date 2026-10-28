#!/bin/bash
# Скрипт случайно выбирает одну из 3 поломок и применяет через values.yaml

RANDOM_BREAK=$((RANDOM % 3 + 1))

echo "=== ВНОСИМ СКРЫТУЮ ПОЛОМКУ ==="

case $RANDOM_BREAK in
  1)
    # Поломка 1: Неверный порт Postgres (5433 вместо 5432)
    sed -i 's/    port: 5432/    port: 5433/' charts/shop/values.yaml
    echo "Применено изменение. Запускаем деплой..."
    ;;
  2)
    # Поломка 2: Неверный лейбл API в разрешении Ingress
    sed -i 's/    allowedApiLabel: "api"/    allowedApiLabel: "api-broken"/' charts/shop/values.yaml
    echo "Применено изменение. Запускаем деплой..."
    ;;
  3)
    # Поломка 3: Отключен DNS
    sed -i 's/    enabled: true/    enabled: false/' charts/shop/values.yaml
    echo "Применено изменение. Запускаем деплой..."
    ;;
esac

helm upgrade shop ./charts/shop -n shop > /dev/null
echo "================================="
echo "СИСТЕМА СЛОМАНА! Ищите проблему в Hubble."