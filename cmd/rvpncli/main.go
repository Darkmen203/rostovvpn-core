package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"
	v2 "github.com/Darkmen203/rostovvpn-core/v2"
)

func main() {
	var cfgPath string
	var enableTun bool
	var disableTun bool
	var mtu int
	var setSystemProxy bool
	var action string

	flag.StringVar(&cfgPath, "config", "", "Path to sing-box base config (required)")
	flag.BoolVar(&enableTun, "enable-tun", false, "Enable TUN/VPN mode")
	flag.BoolVar(&disableTun, "disable-tun", false, "Disable TUN/VPN mode (fallback to proxy)")
	flag.IntVar(&mtu, "mtu", 1450, "TUN MTU")
	flag.BoolVar(&setSystemProxy, "set-system-proxy", false, "Set system proxy (ignored when TUN enabled)")
	flag.StringVar(&action, "action", "start", "start|stop|restart")
	flag.Parse()

	if cfgPath == "" {
		log.Fatalf("missing --config")
	}

	// Меняем опции в рантайме через вашу API
	// Для простоты используем существующие Start/Stop.
	switch action {
	case "start":
		req := &pb.StartRequest{
			ConfigPath:             cfgPath,
			EnableOldCommandServer: true,
			DisableMemoryLimit:     false,
		}
		// При старте проставляем глобальные опции:
		// (В вашем BuildConfig они считываются из RostovVPNOptions)
		// Если хотите, добавьте в v2.ChangeRostovVPNSettings ещё поля.
		if enableTun {
			// Минимально: установим флаг EnableTun в памяти.
			json := fmt.Sprintf(`{"InboundOptions":{"EnableTun":true,"EnableTunService":false,"MTU":%d,"SetSystemProxy":false}}`, mtu)
			_, _ = v2.ChangeRostovVPNSettings(&pb.ChangeRostovVPNSettingsRequest{RostovvpnSettingsJson: json})
		} else if disableTun {
			json := `{"InboundOptions":{"EnableTun":false}}`
			_, _ = v2.ChangeRostovVPNSettings(&pb.ChangeRostovVPNSettingsRequest{RostovvpnSettingsJson: json})
			// можно вернуть прокси, если нужно
			json2 := fmt.Sprintf(`{"InboundOptions":{"SetSystemProxy":%v}}`, setSystemProxy)
			_, _ = v2.ChangeRostovVPNSettings(&pb.ChangeRostovVPNSettingsRequest{RostovvpnSettingsJson: json2})
		}

		if _, err := v2.Start(req); err != nil {
			log.Fatalf("start failed: %v", err)
		}
		// Блокируемся, чтобы процесс держал сервис (по желанию)
		select {}

	case "stop":
		if _, err := v2.Stop(); err != nil {
			log.Fatalf("stop failed: %v", err)
		}

	case "restart":
		if _, err := v2.Restart(&pb.StartRequest{
			ConfigPath:             cfgPath,
			EnableOldCommandServer: true,
		}); err != nil {
			log.Fatalf("restart failed: %v", err)
		}

	default:
		fmt.Println("unknown action:", action)
		os.Exit(2)
	}
}
