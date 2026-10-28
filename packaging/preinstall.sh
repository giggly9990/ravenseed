#!/bin/sh
set -e
if ! getent group ravenseed >/dev/null; then
	groupadd --system ravenseed 2>/dev/null || addgroup -S ravenseed
fi
if ! getent passwd ravenseed >/dev/null; then
	useradd --system --gid ravenseed --home-dir /var/lib/ravenseed --no-create-home --shell /usr/sbin/nologin ravenseed 2>/dev/null ||
		adduser -S -D -H -G ravenseed -h /var/lib/ravenseed -s /sbin/nologin ravenseed
fi
