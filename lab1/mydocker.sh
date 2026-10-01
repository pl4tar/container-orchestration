#!/bin/bash
set -euo pipefail
CGROUP="/sys/fs/cgroup/mydocker"
API="./api"

sudo mkdir -p "$CGROUP"
echo "50000 100000" | sudo tee $CGROUP/cpu.max
echo $((100*1024*1024)) | sudo tee $CGROUP/memory.max
echo 0 | sudo tee $CGROUP/memory.swap.max
echo 20 | sudo tee $CGROUP/pids.max

echo "starting container"
unshare -U -r -m -u -i -n -p -f --mount-proc bash -c '
    set -e
    ip link set lo up
    exec capsh --drop=all --caps= --noamb -- -c "
        exec python3 wrapper.py '"$API"'
    "
' &
unshare_pid=$!
echo "unshare PID: $unshare_pid"
for _ in $(seq 1 20); do
    api_pid=$(pgrep -P "$unshare_pid" || true)
    [ -n "$api_pid" ] && break
    sleep 0.1
done
if [ -z "${api_pid:-}" ]; then
    echo "api did not start" >&2
    kill "$unshare_pid" 2>/dev/null || true
    exit 1
fi
echo "api PID (host): $api_pid"

echo "$api_pid" | sudo tee "$CGROUP/cgroup.procs" >/dev/null

cleanup() {
    trap - INT TERM EXIT

    echo
    echo "Остановка"

    if kill -0 "$UNSHARE_PID" 2>/dev/null; then
        kill -INT "$UNSHARE_PID" 2>/dev/null || true
        wait "$UNSHARE_PID" 2>/dev/null || true
    fi

    sudo ip link del "$HOST_IF" 2>/dev/null || true

    # очистка cgroups после остановки (закомментить если нужно посмотреть всякие ивенты, оом и тд)
    if [[ -d "$CGROUP" ]]; then
        echo 1 | sudo tee "$CGROUP/cgroup.kill" >/dev/null 2>&1 || true
        sudo rmdir "$CGROUP" 2>/dev/null || true
    fi

    echo "Остановлено"
}

trap cleanup INT TERM EXIT

wait "$unshare_pid"