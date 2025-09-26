#!/usr/bin/env bash
set -e
sudo useradd -r -s /usr/sbin/nologin rostovvpn 2>/dev/null || true
sudo mkdir -p /etc/rostovvpn
sudo cp -f dist/rvpncli /usr/local/bin/rvpncli
sudo setcap cap_net_admin,cap_net_raw+eip /usr/local/bin/rvpncli
sudo cp -f packaging/linux/rostovvpn.service /etc/systemd/system/rostovvpn.service
sudo systemctl daemon-reload
sudo systemctl enable rostovvpn.service
echo "Install done. Start with: sudo systemctl start rostovvpn"
