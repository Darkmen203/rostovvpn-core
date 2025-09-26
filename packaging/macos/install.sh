#!/usr/bin/env bash
set -e
sudo install -m 0755 helper/rostovvpn-helper /usr/local/libexec/rostovvpn-helper
sudo install -m 0644 com.rostovvpn.helper.plist /Library/LaunchDaemons/com.rostovvpn.helper.plist
sudo launchctl load -w /Library/LaunchDaemons/com.rostovvpn.helper.plist
echo "Helper installed. Socket: /var/run/rostovvpn.sock"
