#!/bin/bash
set -euo pipefail

# Скрипт для создания нового релиза (тега) на GitHub
# Использование: bash scripts/make-release.sh [version]
# Пример: bash scripts/make-release.sh v1.0.12

info() { printf "\033[0;36m[INFO]\033[0m  %s\n" "$*"; }
ok()   { printf "\033[0;32m[OK]\033[0m    %s\n" "$*"; }
die()  { printf "\033[0;31m[ERROR]\033[0m %s\n" "$*" >&2; exit 1; }

# Проверка наличия грязных файлов
if ! git diff-index --quiet HEAD --; then
    die "У вас есть незакоммиченные изменения. Сначала сделайте коммит."
fi

# Получаем текущую ветку
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_BRANCH" != "main" ]; then
    warn "Вы находитесь не в ветке main ($CURRENT_BRANCH). Продолжить? (y/n)"
    read -r response
    if [ "$response" != "y" ]; then exit 1; fi
fi

# Получаем последнюю версию, если не указана
if [ $# -eq 0 ]; then
    LATEST_TAG=$(git tag -l "v*" | sort -V | tail -n1)
    if [ -z "$LATEST_TAG" ]; then
        NEXT_TAG="v1.0.0"
    else
        # Инкремент последней цифры (v1.0.11 -> v1.0.12)
        BASE=$(echo "$LATEST_TAG" | cut -d. -f1-2)
        PATCH=$(echo "$LATEST_TAG" | cut -d. -f3)
        NEXT_TAG="${BASE}.$((PATCH + 1))"
    fi
    info "Последняя версия: $LATEST_TAG. Будет создана: $NEXT_TAG"
    echo -n "Продолжить? (y/n): "
    read -r response
    if [ "$response" != "y" ]; then exit 1; fi
else
    NEXT_TAG="$1"
fi

info "Отправка последних изменений в main..."
git push origin main

info "Создание тега $NEXT_TAG (сейчас откроется текстовый редактор для ввода описания)..."
# Флаг -a заставляет git создать аннотированный тег и открывает текстовый редактор
git tag -a "$NEXT_TAG"

info "Отправка тега на GitHub..."
git push origin "$NEXT_TAG"

ok "Тег $NEXT_TAG отправлен! GitHub Actions соберет релиз через 3-5 минут."
