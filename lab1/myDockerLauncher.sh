#!/bin/bash

# Очистка предыдущего запуска
systemctl --user stop lab1-memory.scope

systemd-run \
--user \
--scope \
--unit=lab1-memory \
-p MemoryMax=128M \
-p MemorySwapMax=0 \
-p CPUQuota=50% \
-p TasksMax=64 \
./mydocker.sh
