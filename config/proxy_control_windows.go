//go:build windows

package config

import (
	"os/exec"
	"syscall"
)

const (
	internetOptionSettingsChanged = 39
	internetOptionRefresh         = 37
)

var (
	wininet                   = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOptionW    = wininet.NewProc("InternetSetOptionW")
	internetSettingsRegSubKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
)

func internetSetOption(option uintptr) {
	// InternetSetOptionW(NULL, option, NULL, 0)
	_, _, _ = procInternetSetOptionW.Call(0, option, 0, 0)
}

func ProxyOff() error {
	var lastErr error
	run := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		if err := cmd.Run(); err != nil {
			lastErr = err
		}
	}

	// WinINet (HKCU): отключить прокси и очистить значения
	run("reg", "add", internetSettingsRegSubKey,
		"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
	run("reg", "delete", internetSettingsRegSubKey,
		"/v", "ProxyServer", "/f")
	run("reg", "delete", internetSettingsRegSubKey,
		"/v", "AutoConfigURL", "/f")
	run("reg", "delete", internetSettingsRegSubKey,
		"/v", "ProxyOverride", "/f")

	// Оповестить WinINet-клиентов о смене настроек
	internetSetOption(internetOptionSettingsChanged)
	internetSetOption(internetOptionRefresh)

	// WinHTTP (служебный прокси для сервисов) — best effort
	run("netsh", "winhttp", "reset", "proxy")

	return lastErr
}
