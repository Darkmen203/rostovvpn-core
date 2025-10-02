// cli/cmdroot/cmdroot.go
package cmdroot

import (
	"fmt"
	"os"
	"strings"

	"github.com/Darkmen203/rostovvpn-core/config"
	v2 "github.com/Darkmen203/rostovvpn-core/v2"
)

func usage() {
	fmt.Println(`RostovVPNCli
		Usage:
		RostovVPNCli tunnel run
		RostovVPNCli tunnel install
		RostovVPNCli tunnel start
		RostovVPNCli tunnel stop
		RostovVPNCli tunnel uninstall
		RostovVPNCli tunnel exit
		RostovVPNCli tunnel deactivate-force
			RostovVPNCli proxy off

			Notes:
		- "run" запускает service.Run() (под менеджером сервисов).
		- "install/start/stop/uninstall" проксируются в kardianos/service.
		- "exit" шлёт gRPC Exit() — попросить сервис завершиться.
		- "deactivate-force" мягко остановит gRPC, затем приберёт процесс/TUN по ОС.
		- "proxy off" отключает системный прокси (best-effort на Win/macOS/Linux).
		`)
}

func Run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	switch args[0] {
	case "tunnel":
		return handleTunnel(args[1:])
	case "proxy":
		return handleProxy(args[1:])
	default:
		usage()
		return 2
	}
}

func handleTunnel(args []string) int {
	if len(args) == 0 {
		// по умолчанию: старт сервиса
		code, out := v2.StartTunnelService("start")
		if strings.TrimSpace(out) != "" {
			fmt.Println(strings.TrimSpace(out))
		}
		return code
	}

	cmd := args[0]
	switch cmd {
	case "run", "start", "stop", "install", "uninstall":
		code, out := v2.StartTunnelService(cmd)
		if strings.TrimSpace(out) != "" {
			fmt.Println(strings.TrimSpace(out))
		}
		return code

	case "exit":
		// мягкое завершение сервиса (через gRPC Exit)
		ok, err := config.ExitTunnelService()
		if err != nil {
			fmt.Println("exit error:", err)
			return 1
		}
		if !ok {
			fmt.Println("exit: not ok")
			return 1
		}
		fmt.Println("exit: ok")
		return 0

	case "deactivate-force":
		// сначала мягко (Stop), затем силовая зачистка
		if ok, _ := config.DeactivateTunnelService(); !ok {
			// игнорируем ошибку; перейдём к force
		}
		ok, err := config.DeactivateTunnelServiceForce()
		if err != nil {
			fmt.Println("force deactivate error:", err)
			return 1
		}
		if !ok {
			fmt.Println("force deactivate: port still busy")
			return 1
		}
		fmt.Println("force deactivate: done")
		return 0

	default:
		usage()
		return 2
	}
}

func handleProxy(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "off":
		if err := config.ProxyOff(); err != nil {
			fmt.Println("proxy off error:", err)
			return 1
		}
		fmt.Println("proxy off: ok")
		return 0
	default:
		usage()
		return 2
	}
}

// Для совместимости: вспомогательная функция — выход с кодом
func Main() {
	os.Exit(Run(os.Args[1:]))
}
