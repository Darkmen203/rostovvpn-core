//go:build linux && !android

package config

import (
	"os/exec"
)

func ProxyOff() error {
	var lastErr error
	run := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		if err := cmd.Run(); err != nil {
			lastErr = err
		}
	}
	// GNOME: системный прокси через gsettings
	run("gsettings", "set", "org.gnome.system.proxy", "mode", "none")
	// Очистка (если кто-то оставил домены/параметры) — не обязательно
	// run("gsettings", "reset-recursively", "org.gnome.system.proxy")
	return lastErr
}
