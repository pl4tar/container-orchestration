#!/usr/bin/env bash
# mydocker.sh — запуск сервиса api в собственных namespaces,
# с cgroup-лимитами и урезанными правами. Аналог docker run,
# собранный из частей 2-4.
#
#   ./mydocker.sh
#
set -euo pipefail

# ---- параметры ----------------------------------------------------------
CG_NAME=${CG_NAME:-mydocker}
CG=/sys/fs/cgroup/$CG_NAME
API=${API:-$HOME/api}
SECCOMP_RUN=${SECCOMP_RUN:-$HOME/seccomp-run}
HOSTNAME_IN=${HOSTNAME_IN:-mybox}

MEM_MAX=${MEM_MAX:-64M}
CPU_MAX=${CPU_MAX:-50000 100000}
PIDS_MAX=${PIDS_MAX:-20}

export API SECCOMP_RUN
# ---- проверки перед стартом --------------------------------------------
for f in "$API" "$SECCOMP_RUN"; do
	[[ -x $f ]] || { echo "нет исполняемого файла: $f" >&2; exit 1; }
done

if [[ $(sysctl -n kernel.apparmor_restrict_unprivileged_userns 2>/dev/null || echo 0) == 1 ]]; then
	echo "user namespaces запрещены AppArmor; см. sysctl kernel.apparmor_restrict_unprivileged_userns" >&2
	exit 1
fi

# ---- уборка при выходе --------------------------------------------------
# rmdir на cgroup срабатывает, только когда в ней не осталось процессов,
# поэтому сначала возвращаем себя в корневую cgroup.
cleanup() {
	echo $$ | sudo tee /sys/fs/cgroup/cgroup.procs >/dev/null 2>&1 || true
	sudo rmdir "$CG" 2>/dev/null || true
}
trap cleanup EXIT

# ---- 1. cgroup и лимиты -------------------------------------------------
# Контроллеры должны быть разрешены родителем в cgroup.subtree_control,
# иначе файлов memory.max / cpu.max / pids.max в нашем каталоге не появится.
echo "+memory +pids +cpu" | sudo tee /sys/fs/cgroup/cgroup.subtree_control >/dev/null 2>&1 || true
sudo mkdir -p "$CG"

# TODO(1): выставь три лимита и запрет swap.
#   Файлы: memory.max ($MEM_MAX), memory.swap.max (0),
#          cpu.max ($CPU_MAX), pids.max ($PIDS_MAX).
#   Форма записи — как в Части 3: echo ЗНАЧЕНИЕ | sudo tee ФАЙЛ >/dev/null\

echo $MEM_MAX | sudo tee $CG/memory.max >/dev/null
echo 0 | sudo tee $CG/memory.swap.max >/dev/null
echo "$CPU_MAX" | sudo tee $CG/cpu.max >/dev/null
echo $PIDS_MAX | sudo tee $CG/pids.max >/dev/null

# ---- 2. войти в cgroup --------------------------------------------------
# Делается ДО запуска сервиса: начисления памяти не переносятся между
# cgroup, а дочерние процессы членство наследуют.
#
# TODO(2): помести текущий процесс ($$) в $CG/cgroup.procs
echo $$ | sudo tee "$CG/cgroup.procs" >/dev/null

echo "cgroup: $(cat /proc/self/cgroup)"
echo "лимиты: mem=$(cat "$CG/memory.max" 2>/dev/null) cpu=$(cat "$CG/cpu.max" 2>/dev/null) pids=$(cat "$CG/pids.max" 2>/dev/null)"

# ---- 3. namespaces + права + запуск ------------------------------------
# TODO(3): цепочка из шести namespaces, настройки окружения, сброса
# capabilities, seccomp-фильтра и самого сервиса.
#
#   unshare <шесть namespace-флагов> bash -c '
#       hostname ...; ip link set lo up
#       exec capsh --drop=... -- -c "exec SECCOMP_RUN API"
#   '
#
# На каждом переходе exec: процесс должен остаться одним и тем же,
# чтобы api получил PID 1, а вместе с ним cgroup, namespaces,
# опущенный bounding set и загруженный фильтр.

unshare --user --map-root-user --pid --fork --mount --mount-proc --net --uts --ipc bash -c 'hostname mybox; ip link set lo up;exec capsh --drop=all -- -c " exec $SECCOMP_RUN $API"'
