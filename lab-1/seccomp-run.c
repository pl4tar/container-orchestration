// seccomp-run — навешивает seccomp-фильтр и запускает указанную программу.
//
//   ./seccomp-run <программа> [аргументы...]
//
// Фильтр наследуется через exec(), поэтому целевая программа стартует
// уже под ним. Снять его нельзя: seccomp-фильтр необратим.
//
// Сборка: gcc -o seccomp-run seccomp-run.c -lseccomp

#include <errno.h>
#include <seccomp.h>
#include <stdio.h>
#include <string.h>
#include <sys/prctl.h>
#include <unistd.h>

int main(int argc, char *argv[]) {
	if (argc < 2) {
		fprintf(stderr, "usage: %s <program> [args...]\n", argv[0]);
		return 2;
	}

	// 1. no_new_privs. Без этого флага ядро не даст непривилегированному
	// процессу поставить фильтр (EACCES). Флаг запрещает повышение привилегий
	// через setuid-бинарники и, как и сам фильтр, необратим.
	if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) < 0) {
		fprintf(stderr, "prctl(NO_NEW_PRIVS): %s\n", strerror(errno));
		return 1;
	}

	// 2. Контекст фильтра. SCMP_ACT_ALLOW — blacklist: разрешено всё, кроме
	// явно описанного ниже. Строгий вариант — SCMP_ACT_ERRNO(EPERM) по
	// умолчанию (whitelist), но тогда надо перечислить все нужные вызовы.
	scmp_filter_ctx ctx = seccomp_init(SCMP_ACT_ALLOW);
	if (!ctx) {
		fprintf(stderr, "seccomp_init failed\n");
		return 1;
	}

	// 3. Правила. Последний аргумент — число условий на аргументы вызова
	// (фильтровать можно и по ним, но только по скалярам: разыменовать
	// указатель BPF-программа не может — иначе получилась бы гонка TOCTOU).
	//
	// Вердикт выбирается осознанно:
	//   SCMP_ACT_ERRNO(EPERM) — вызов не выполняется, программа получает ошибку;
	//   SCMP_ACT_KILL_PROCESS — процесс убивается на месте (SIGSYS);
	//   SCMP_ACT_LOG          — разрешить, но записать в аудит (для отладки).
	int rc = seccomp_rule_add(ctx, SCMP_ACT_ERRNO(EPERM), SCMP_SYS(clock_settime), 0);
	if (rc < 0) {
		fprintf(stderr, "rule clock_settime: %s\n", strerror(-rc));
		return 1;
	}

	// На arm64 отдельного chmod(2) не существует: ядро оставило только
	// *at-формы, и chmod(1) из coreutils уходит в fchmodat(AT_FDCWD, ...).
	// Именно поэтому seccomp-профили привязаны к архитектуре.
	rc = seccomp_rule_add(ctx, SCMP_ACT_ERRNO(EPERM), SCMP_SYS(fchmodat), 0);
	if (rc < 0) {
		fprintf(stderr, "rule fchmodat: %s\n", strerror(-rc));
		return 1;
	}

	// Ядра 6.6+ вместе со свежей glibc используют fchmodat2 — новую форму
	// вызова, куда наконец завезли флаги. Старое правило её не ловит, поэтому
	// закрывать нужно обе: классическая беда seccomp-профилей, когда ядро
	// добавляет вызов с прежней семантикой, а профиль о нём не знает.
	rc = seccomp_rule_add(ctx, SCMP_ACT_ERRNO(EPERM), SCMP_SYS(fchmodat2), 0);
	if (rc < 0) {
		fprintf(stderr, "rule fchmodat2: %s\n", strerror(-rc));
		return 1;
	}

	// 4. Загрузка фильтра в ядро. До этого момента всё жило в памяти
	// процесса как черновик.
	rc = seccomp_load(ctx);
	if (rc < 0) {
		fprintf(stderr, "seccomp_load: %s\n", strerror(-rc));
		return 1;
	}

	seccomp_release(ctx);

	execvp(argv[1], &argv[1]);

	fprintf(stderr, "exec %s: %s\n", argv[1], strerror(errno));
	return 127;
}
