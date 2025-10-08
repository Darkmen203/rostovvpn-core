//go:build !windows

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func ExecuteCmd(executablePath string, background bool, args ...string) (string, error) {
	cwd := filepath.Dir(executablePath)

	// AppImage: оставляем вашу семантику
	if appimage := os.Getenv("APPIMAGE"); appimage != "" {
		executablePath = appimage
		if !background {
			return "Fail", fmt.Errorf("AppImage cannot have service")
		}
	}

	full := append([]string{executablePath}, args...)

	tryRun := func(cmdline []string) error {
		if len(cmdline) == 0 {
			return errors.New("empty command")
		}
		cmd := exec.Command(cmdline[0], cmdline[1:]...)
		cmd.Dir = cwd
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Printf("Running command: %v\n", cmdline)
		if background {
			return cmd.Start()
		}
		return cmd.Run()
	}

	// Собираем команды по платформе
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		// 1) Нативная эскалация через AppleScript (GUI prompt)
		// do shell script "...</escaped...>" with administrator privileges
		esc := func(s string) string { return strings.ReplaceAll(s, `"`, `\"`) }
		// Команду исполним через sh -c, чтобы проще экранировать аргументы
		shCmd := "exec " + shellJoin(full)
		osascript := []string{"/usr/bin/osascript", "-e",
			fmt.Sprintf(`do shell script "%s" with administrator privileges prompt "RostovVPN needs root for tunneling."`, esc(shCmd)),
		}
		candidates = append(candidates, osascript)

		// 2) Если есть cocoasudo — тоже ок
		candidates = append(candidates, append([]string{"cocoasudo", "--prompt=RostovVPN needs root for tunneling.", executablePath}, args...))

		// 3) sudo с askpass (если задан SUDO_ASKPASS)
		if os.Getenv("SUDO_ASKPASS") != "" {
			candidates = append(candidates, append([]string{"sudo", "-A", executablePath}, args...))
		}
		// 4) обычный sudo (TTY)
		candidates = append(candidates, append([]string{"sudo", executablePath}, args...))

	default: // linux и прочее unix
		cwdQuoted := shellJoin([]string{cwd})
		fullShell := shellJoin(full)
		// 1) pkexec (polkit GUI) — оборачиваем, чтобы сохранить рабочий каталог и путь к библиотекам
		pkexecCmd := []string{"pkexec", "sh", "-c", fmt.Sprintf("cd %s && export LD_LIBRARY_PATH=%s/lib:$LD_LIBRARY_PATH && exec %s", cwdQuoted, cwdQuoted, fullShell)}
		candidates = append(candidates, pkexecCmd)
		// 2) sudo -A (GUI askpass, если есть)
		if os.Getenv("SUDO_ASKPASS") != "" {
			candidates = append(candidates, append([]string{"sudo", "-A", executablePath}, args...))
		}
		// 3) обычный sudo
		candidates = append(candidates, append([]string{"sudo", executablePath}, args...))
		// 4) графические фронтенды (устаревшие, но иногда встречаются)
		candidates = append(candidates, append([]string{"gksudo", executablePath}, args...))
		candidates = append(candidates, append([]string{"kdesu", executablePath}, args...))
		// 5) xterm → sudo (на минимальных окружениях)
		candidates = append(candidates, []string{"xterm", "-e", "sudo", shellJoin(append([]string{executablePath}, args...))})
	}

	var lastErr error
	for _, c := range candidates {
		// Пропускаем варианты, которых нет в $PATH (кроме абсолютных путей)
		if !strings.HasPrefix(c[0], "/") {
			if _, err := exec.LookPath(c[0]); err != nil {
				lastErr = err
				continue
			}
		}
		if err := tryRun(c); err == nil {
			return "Ok", nil
		} else {
			lastErr = err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no elevation candidate executed")
	}
	return "", fmt.Errorf("failed to acquire admin rights: %w", lastErr)
}

// shellJoin аккуратно собирает команду/аргументы в одну строку для sh -c
// с безопасным экранированием пробелов и кавычек.
func shellJoin(parts []string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			out = append(out, "''")
			continue
		}
		// Экранируем одиночные кавычки: ' -> '\''  (POSIX способ)
		if strings.IndexByte(p, '\'') >= 0 {
			p = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
		} else if strings.ContainsAny(p, " \t\n\\\"$`") {
			p = "'" + p + "'"
		}
		out = append(out, p)
	}
	return strings.Join(out, " ")
}
