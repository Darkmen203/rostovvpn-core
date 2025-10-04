package v2

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kardianos/service"
	"google.golang.org/grpc"
)

var logger service.Logger

type rostovvpnNext struct {
	srv *grpc.Server
}

var port int = 18020

func (m *rostovvpnNext) Start(s service.Service) error {
	srv, err := StartTunnelGrpcServer(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	m.srv = srv
	return nil
}

func (m *rostovvpnNext) Stop(s service.Service) error {
	// 1) остановить ядро (Box/TUN/маршруты)
	_, _ = Stop()
	time.Sleep(150 * time.Millisecond)

	// 2) корректно погасить gRPC-сервер, чтобы освободить 18020
	if m.srv != nil {
		done := make(chan struct{})
		go func() { m.srv.GracefulStop(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			m.srv.Stop() // жёстко
		}
		m.srv = nil
	}
	return nil
}

func getCurrentExecutableDirectory() string {
	executablePath, err := os.Executable()
	if err != nil {
		return ""
	}

	// Extract the directory (folder) containing the executable
	executableDirectory := filepath.Dir(executablePath)

	return executableDirectory
}

func StartTunnelService(goArg string) (int, string) {
	svcConfig := &service.Config{
		Name:        "RostovVPNTunnelService",
		DisplayName: "RostovVPN Tunnel Service",
		Arguments:   []string{"tunnel", "run"},
		Description: "This is a bridge for tunnel",
		Option: map[string]interface{}{
			"RunAtLoad":        true,
			"WorkingDirectory": getCurrentExecutableDirectory(),
			// для systemd:
			"Restart":    "on-failure",
			"RestartSec": "2",
			// запустить как system service (root), а не user service:
			"UserService": false,
		},
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// ВАЖНО: короткое замыкание по занятому порту — только для интентов "start" или пустого (ручной старт)
	if goArg == "" || goArg == "start" {
		if isPortBusy(addr, 500*time.Millisecond) {
			return 0, "Tunnel Service already running (port busy)"
		}
	}

	prg := &rostovvpnNext{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		// log.Printf("Error: %v", err)
		return 1, fmt.Sprintf("Error: %v", err)
	}

	if len(goArg) > 0 && goArg != "run" {
		// Перед установкой/стартом — убедимся, что есть override без сетевой изоляции.
		if (goArg == "install" || goArg == "start") && runtime.GOOS == "linux" {
			if err := ensureSystemdOverride(svcConfig.Name, getCurrentExecutableDirectory()); err != nil {
				log.Printf("ensureSystemdOverride: %v", err)
			}
		}
		return control(s, goArg)
	}

	logger, err = s.Logger(nil)
	if err != nil {
		log.Printf("Error: %v", err)
	}
	err = s.Run()
	if err != nil {
		logger.Error(err)
		return 3, fmt.Sprintf("Error: %v", err)
	}
	return 0, ""
}

// Создаём drop-in /etc/systemd/system/<name>.service.d/override.conf
// чтобы sing-box запускался от root, без PrivateNetwork/RestrictNamespaces,
// и видел полноценный сетевой namespace хоста.
func ensureSystemdOverride(unitName, workDir string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	dir := fmt.Sprintf("/etc/systemd/system/%s.service.d", unitName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	override := `[Service]
		User=root
		Group=root
		WorkingDirectory=` + workDir + `
		PrivateNetwork=no
		RestrictNamespaces=no
		NoNewPrivileges=false
	`
	path := filepath.Join(dir, "override.conf")
	if err := os.WriteFile(path, []byte(override), 0644); err != nil {
		return err
	}
	// перезагрузить конфигурацию systemd
	_ = exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func control(s service.Service, goArg string) (int, string) {
	dolog := false
	var err error
	status, serr := s.Status()
	if dolog {
		fmt.Printf("Current Status: %+v %+v!\n", status, serr)
	}
	switch goArg {
	case "uninstall":
		if status == service.StatusRunning {
			s.Stop()
		}
		if dolog {
			fmt.Printf("Tunnel Service Uninstalled Successfully.\n")
		}
		err = s.Uninstall()
	case "start":
		// убедимся, что override на месте и перечитан
		if runtime.GOOS == "linux" {
			_ = exec.Command("systemctl", "daemon-reload").Run()
		}
		if status == service.StatusRunning {
			if dolog {
				fmt.Printf("Tunnel Service Already Running.\n")
			}
			return 0, "Tunnel Service Already Running."
		} else if status == service.StatusUnknown {
			s.Uninstall()
			s.Install()
			if runtime.GOOS == "linux" {
				_ = exec.Command("systemctl", "daemon-reload").Run()
			}
			status, serr = s.Status()
			if dolog {
				fmt.Printf("Check status again: %+v %+v!", status, serr)
			}
		}
		if status != service.StatusRunning {
			err = s.Start()
		}
	case "install":
		s.Uninstall()
		err = s.Install()
		if runtime.GOOS == "linux" {
			_ = exec.Command("systemctl", "daemon-reload").Run()
		}
		status, serr = s.Status()
		if dolog {
			fmt.Printf("Check Status Again: %+v %+v", status, serr)
		}
		if status != service.StatusRunning {
			err = s.Start()
		}
	case "stop":
		if status == service.StatusStopped {
			if dolog {
				fmt.Printf("Tunnel Service Already Stopped.\n")
			}
			return 0, "Tunnel Service Already Stopped."
		}
		err = s.Stop()
	default:
		err = service.Control(s, goArg)
	}
	if err == nil {
		out := fmt.Sprintf("Tunnel Service %sed Successfully.", goArg)
		if dolog {
			fmt.Printf(out)
		}
		return 0, out
	} else {
		out := fmt.Sprintf("Error: %v", err)
		if dolog {
			log.Printf(out)
		}
		return 2, out
	}
}

func isPortBusy(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err == nil {
		_ = conn.Close()
		return true
	}
	return false
}
