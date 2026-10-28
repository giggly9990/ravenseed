#!/bin/sh
set -e
# Upgrades also run this; only stop the service on removal.
case "$1" in
remove | purge | 0) ;;
*) exit 0 ;;
esac
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
	systemctl disable --now ravenseed >/dev/null 2>&1 || true
fi
