package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"
	v2 "github.com/Darkmen203/rostovvpn-core/v2"
)

func stopFilePath() string {
	// единое место stop-файла: %APPDATA%\RostovVPN\rvpncli.stop на Win,
	// а на *nix — ~/.config/rostovvpn/rvpncli.stop (или рабочая директория core)
	base := ""
	if h, err := os.UserConfigDir(); err == nil && h != "" {
		base = filepath.Join(h, "RostovVPN")
	} else {
		base = filepath.Join(os.TempDir(), "RostovVPN")
	}
	_ = os.MkdirAll(base, 0o755)
	return filepath.Join(base, "rvpncli.stop")
}

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

		// Убираем возможный старый stop-файл
		_ = os.Remove(stopFilePath())
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
		for {
			time.Sleep(500 * time.Millisecond)
			if _, err := os.Stat(stopFilePath()); err == nil {
				// файл появился → останавливаем core и выходим
				_, _ = v2.Stop()
				_ = os.Remove(stopFilePath())
				break
			}
		}

	case "stop":
		// Создаём stop-файл, чтобы «стартующий» rvpncli корректно остановил core и вышел
		f, err := os.Create(stopFilePath())
		if err == nil {
			f.Close()
		}
		// на всякий случай (если кто-то держит core не через нашу блокировку)
		_, _ = v2.Stop()

	case "restart":
		if cfgPath == "" {
			log.Fatalf("missing --config")
		}
		_, _ = v2.Stop()
		if _, err := v2.Restart(&pb.StartRequest{
			ConfigPath:             cfgPath,
			EnableOldCommandServer: true,
		}); err != nil {
			log.Fatalf("restart failed: %v", err)
		}

	default:
		log.Fatalf("unknown action: %s", action)
	}
}
