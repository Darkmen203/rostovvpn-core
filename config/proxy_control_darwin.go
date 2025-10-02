//go:build darwin

package config

import (
	"bufio"
	"bytes"
	"os/exec"
	"strings"
)

func ProxyOff() error {
	var lastErr error

	// Получаем список всех сервисов (Wi-Fi, Ethernet, etc.)
	out, err := exec.Command("/usr/sbin/networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return err
	}

	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		svc := strings.TrimSpace(sc.Text())
		if svc == "" {
			continue
		}
		// Строка-примечание: "An asterisk (*) denotes that a network service is disabled."
		if strings.HasPrefix(svc, "An asterisk") {
			continue
		}
		// Disabled service помечается '*Name' — удалим звёздочку
		svc = strings.TrimPrefix(svc, "*")

		// Для каждого сервиса выключаем web/secure/auto proxy (best-effort)
		for _, args := range [][]string{
			{"-setwebproxystate", svc, "off"},
			{"-setsecurewebproxystate", svc, "off"},
			{"-setautoproxystate", svc, "off"},
		} {
			if err := exec.Command("/usr/sbin/networksetup", args...).Run(); err != nil {
				lastErr = err
			}
		}
	}
	_ = sc.Err()
	return lastErr
}
