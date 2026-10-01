#!/usr/bin/env python3
"""Применяет seccomp-фильтр к себе и exec'ает api."""
import os
import sys

import seccomp  # python3-seccomp / python-libseccomp
import errno

API = sys.argv[1] if len(sys.argv) > 1 else "./api"


def main():
    

    # было: defaction=KILL_PROCESS, список allowed
    # стало: defaction=ALLOW — всё, что не запрещено, работает
    f = seccomp.SyscallFilter(defaction=seccomp.ALLOW)

    # запрещаем точечно, с возвратом ошибки — не убийством
    for name in ["ptrace", "mount", "umount2", "kexec_load",
             "keyctl", "add_key", "bpf", "userfaultfd",
             "reboot", "swapon", "swapoff",
             "init_module", "finit_module", "delete_module"]:
        f.add_rule(seccomp.ERRNO(errno.EPERM), name)

    f.load()
    os.execv(API, [API])


if __name__ == "__main__":
    main()