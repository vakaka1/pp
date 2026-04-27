#!/bin/bash
GREEN='\033[0;32m'; CYAN='\033[0;36m'; NC='\033[0m'
clear_line() { printf "\r\033[K"; }
info()  { clear_line; echo -e "${CYAN}[INFO]${NC}  $*"; }

start_progress() {
    local msg="$1"
    (
        local width=20
        local pos=0
        local dir=1
        while true; do
            local bar=""
            for ((i=0; i<width; i++)); do
                if [ "$i" -eq "$pos" ]; then bar="${bar}━"
                elif [ "$i" -eq "$((pos-1))" ] || [ "$i" -eq "$((pos+1))" ]; then bar="${bar}─"
                else bar="${bar}·"
                fi
            done
            # Write to stderr to avoid stdout buffering issues if any
            printf "\r\033[K${GREEN}  [%s] %s...${NC}" "$bar" "$msg" >&2
            pos=$((pos + dir))
            if [ "$pos" -ge "$((width-1))" ]; then dir=-1; elif [ "$pos" -le 0 ]; then dir=1; fi
            sleep 0.08
        done
    ) &
    PROGRESS_PID=$!
}

stop_progress() {
    if [ -n "${PROGRESS_PID:-}" ]; then
        kill "$PROGRESS_PID" 2>/dev/null || true
        wait "$PROGRESS_PID" 2>/dev/null || true
        printf "\r\033[K" >&2
        unset PROGRESS_PID
    fi
}

start_progress "Downloading"
sleep 1
info "Downloading a..."
sleep 1
info "Downloading b..."
sleep 1
stop_progress
echo "Done"
