#!/bin/sh
set -e
install -d -o ravenseed -g ravenseed -m 0750 /var/lib/ravenseed
chown root:ravenseed /etc/ravenseed/ravenseed.env
chmod 0640 /etc/ravenseed/ravenseed.env
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
	systemctl daemon-reload
	if systemctl is-active --quiet ravenseed; then
		systemctl restart ravenseed
	else
		echo "Edit /etc/ravenseed/ravenseed.env, then: systemctl enable --now ravenseed"
	fi
fi
