#!/bin/bash
set -euo pipefail

GREEN='\033[0;32m'
NC='\033[0m'

{
    sleep 3
    exit 0
} >/dev/null 2>&1 &
PID=$!

WIDTH=20
POS=0
DIR=1
while kill -0 "$PID" 2>/dev/null; do
    BAR=""
    for ((i=0; i<WIDTH; i++)); do
        if [ "$i" -eq "$POS" ]; then
            BAR="${BAR}━"
        elif [ "$i" -eq "$((POS-1))" ] || [ "$i" -eq "$((POS+1))" ]; then
            BAR="${BAR}─"
        else
            BAR="${BAR}·"
        fi
    done
    printf "\r${GREEN}  [%s] Установка системных зависимостей...${NC}" "$BAR"
    POS=$((POS + DIR))
    if [ "$POS" -ge "$((WIDTH-1))" ]; then DIR=-1; elif [ "$POS" -le 0 ]; then DIR=1; fi
    sleep 0.08
done
wait "$PID"
printf "\r\033[K"
echo "Done."
