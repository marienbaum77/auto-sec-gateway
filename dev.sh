#!/bin/bash
set -e
if ! docker system info | grep -q "ghcr.io"; then
    echo "🔑 Авторизация в GHCR..."
    if [ -z "$GITHUB_TOKEN" ]; then
        echo "❌ Ошибка: Переменная GITHUB_TOKEN не задана. Сделай export GITHUB_TOKEN=..."
        exit 1
    fi
    echo "$GITHUB_TOKEN" | docker login ghcr.io -u marienbaum77 --password-stdin
fi
echo "🚀 Запуск Dev-окружения..."

# 1. Собираем образ и пушим с тегом 'dev'
echo "📦 Сборка образа..."
cd manager
docker build -t ghcr.io/marienbaum77/auto-sec-gateway/manager:dev .
docker push ghcr.io/marienbaum77/auto-sec-gateway/manager:dev
cd ..

# 2. Временно меняем тег образа в kustomize на 'dev'
echo "⚙️ Подготовка манифестов..."
cd k8s/overlays/dev
kustomize edit set image ghcr.io/marienbaum77/auto-sec-gateway/manager=ghcr.io/marienbaum77/auto-sec-gateway/manager:dev
cd ../../..

# 3. Применяем DEV-оверлей напрямую в кластер
echo "🚢 Деплой в кластер..."
kubectl apply -k k8s/overlays/dev

# 4. Принудительный рестарт менеджера для подхвата нового кода
kubectl rollout restart deployment manager -n prod

echo "✅ Dev-окружение обновлено!"