package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Darkmen203/rostovvpn-core/config"
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

func cleanConfigPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "\"")
	path = strings.Trim(path, "'")
	return path
}

func prepareEnvironment(cfgPath string) (string, string, string, string, error) {
	absCfgPath, err := filepath.Abs(cleanConfigPath(cfgPath))
	if err != nil {
		return "", "", "", "", err
	}
	workingDir := filepath.Dir(absCfgPath)
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return "", "", "", "", err
	}
	baseDir := filepath.Dir(workingDir)
	if baseDir == "" || baseDir == "." {
		baseDir = workingDir
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", "", "", "", err
	}
	tempDir := filepath.Join(os.TempDir(), "RostovVPN")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", "", "", "", err
	}
	return absCfgPath, baseDir, workingDir, tempDir, nil
}

func loadSettingsJSON(workingDir string, enableTun, disableTun bool, mtu int, setSystemProxy bool) string {
	prefsPath := filepath.Join(workingDir, "shared_preferences.json")
	options, err := v2.ReadRostovVPNOptionsAt(prefsPath)
	if err != nil {
		log.Printf("warning: failed to read shared preferences: %v", err)
	}
	if options == nil {
		options = config.DefaultRostovVPNOptions()
	}
	if enableTun {
		options.InboundOptions.EnableTun = true
		options.InboundOptions.EnableTunService = false
		if mtu > 0 {
			options.InboundOptions.MTU = uint32(mtu)
		}
		options.InboundOptions.SetSystemProxy = false
	} else if disableTun {
		options.InboundOptions.EnableTun = false
		options.InboundOptions.SetSystemProxy = setSystemProxy
	} else if setSystemProxy {
		options.InboundOptions.SetSystemProxy = true
	}
	data, err := json.Marshal(options)
	if err != nil {
		log.Printf("warning: failed to encode settings: %v", err)
		return ""
	}
	return string(data)
}

func waitClashTCP() {
	deadline := time.Now().Add(10 * time.Second) // под Windows TUN + создание Wintun иногда дольше 3с
	for {
		c, err := net.DialTimeout("tcp", "127.0.0.1:8964", 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			log.Printf("[rvpncli] CommandServer is listening at 127.0.0.1:8964")
			return
		}
		if time.Now().After(deadline) {
			log.Printf("[rvpncli] WARNING: CommandServer not listening. last err: %v", err)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
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
	if enableTun && disableTun {
		log.Fatalf("cannot use --enable-tun and --disable-tun together")
	}

	// Меняем опции в рантайме через вашу API
	// Для простоты используем существующие Start/Stop.
	switch action {
	case "start":
		absCfg, baseDir, workingDir, tempDir, err := prepareEnvironment(cfgPath)
		if err != nil {
			log.Fatalf("prepare environment failed: %v", err)
		}

		if err := v2.Setup(baseDir, workingDir, tempDir, 0, false); err != nil {
			log.Fatalf("[rvpncli] v2.Setup failed: %v", err)
		}

		log.Printf("[rvpncli] start: cfg=%s, workingDir=%s", absCfg, workingDir)

		if settingsJSON := loadSettingsJSON(workingDir, enableTun, disableTun, mtu, setSystemProxy); settingsJSON != "" {
			log.Printf("[rvpncli] settingsJSON len=%d", len(settingsJSON))
			if _, err := v2.ChangeRostovVPNSettings(&pb.ChangeRostovVPNSettingsRequest{RostovvpnSettingsJson: settingsJSON}); err != nil {
				log.Printf("[rvpncli] change settings failed: %v", err)
			}
		}

		req := &pb.StartRequest{
			ConfigPath:             absCfg,
			EnableOldCommandServer: true,
			DisableMemoryLimit:     false,
		}

		// Убираем возможный старый stop-файл
		_ = os.Remove(stopFilePath())

		if _, err := v2.Start(req); err != nil {
			log.Fatalf("start failed: %v", err)
		}
		log.Printf("[rvpncli] v2.Start OK, waiting CommandServer ...")
		waitClashTCP()

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
		absCfg, baseDir, workingDir, tempDir, err := prepareEnvironment(cfgPath)
		if err != nil {
			log.Fatalf("prepare environment failed: %v", err)
		}
		if err := v2.Setup(baseDir, workingDir, tempDir, 0, false); err != nil {
			log.Fatalf("setup failed: %v", err)
		}
		waitClashTCP() // <<< ждём 127.0.0.1:8964

		if settingsJSON := loadSettingsJSON(workingDir, enableTun, disableTun, mtu, setSystemProxy); settingsJSON != "" {
			if _, err := v2.ChangeRostovVPNSettings(&pb.ChangeRostovVPNSettingsRequest{RostovvpnSettingsJson: settingsJSON}); err != nil {
				log.Printf("change settings failed: %v", err)
			}
		}
		_, _ = v2.Stop()
		if _, err := v2.Restart(&pb.StartRequest{
			ConfigPath:             absCfg,
			EnableOldCommandServer: true,
		}); err != nil {
			log.Fatalf("restart failed: %v", err)
		}

	default:
		log.Fatalf("unknown action: %s", action)
	}
}
