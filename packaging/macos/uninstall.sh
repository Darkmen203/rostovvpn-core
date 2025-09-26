#!/usr/bin/env bash
sudo launchctl unload -w /Library/LaunchDaemons/com.rostovvpn.helper.plist || true
sudo rm -f /Library/LaunchDaemons/com.rostovvpn.helper.plist
sudo rm -f /usr/local/libexec/rostovvpn-helper
sudo rm -f /var/run/rostovvpn.sock
