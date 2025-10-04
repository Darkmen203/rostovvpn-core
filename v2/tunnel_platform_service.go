package v2

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kardianos/service"
	"google.golang.org/grpc"
)

var logger service.Logger

// настройки nft/policy routing (совпадают с GUI-частью)
const (
	nftTable = "rostovvpn"
	nftChain = "prerouting"
	fwMark   = "0x1"
	rtTable  = "100"
)

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

// ===== Linux TPROXY helpers =====
func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w (%s)", name, args, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
func cmdExists(ctx context.Context, name string, args ...string) bool {
	return exec.CommandContext(ctx, name, args...).Run() == nil
}

// EnsureTPROXY — ставит policy routing и nft tproxy. Вызывать перед запуском tproxy-конфига.
func EnsureTPROXY(tproxyPort, dnsLocalPort int, enableDNSRedirect bool) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	// 1) policy routing
	if !cmdExists(ctx, "sh", "-c", "ip rule show | grep -q 'fwmark "+fwMark+" lookup "+rtTable+"'") {
		if err := runCmd(ctx, "ip", "rule", "add", "fwmark", fwMark, "lookup", rtTable); err != nil {
			return err
		}
	}
	if !cmdExists(ctx, "sh", "-c", "ip route show table "+rtTable+" | grep -q 'local 0.0.0.0/0 dev lo'") {
		if err := runCmd(ctx, "ip", "route", "add", "local", "0.0.0.0/0", "dev", "lo", "table", rtTable); err != nil {
			return err
		}
	}
	// IPv6 (best-effort)
	_ = runCmd(ctx, "ip", "-6", "rule", "add", "fwmark", fwMark, "lookup", rtTable)
	_ = runCmd(ctx, "ip", "-6", "route", "add", "local", "::/0", "dev", "lo", "table", rtTable)

	// 2) nftables: таблица/цепочка
	_ = runCmd(ctx, "nft", "create", "table", "inet", nftTable)
	_ = runCmd(ctx, "nft", "add", "chain", "inet", nftTable, nftChain, "{ type filter hook prerouting priority mangle; policy accept; }")

	// 3) правила TPROXY TCP/UDP
	tcpRule := fmt.Sprintf("ip protocol tcp tproxy to :%d meta mark set %s accept", tproxyPort, fwMark)
	udpRule := fmt.Sprintf("ip protocol udp tproxy to :%d meta mark set %s accept", tproxyPort, fwMark)
	if !cmdExists(ctx, "sh", "-c", "nft list ruleset | grep -Fq '"+tcpRule+"'") {
		if err := runCmd(ctx, "nft", "add", "rule", "inet", nftTable, nftChain,
			"ip", "protocol", "tcp", "tproxy", "to", fmt.Sprintf(":%d", tproxyPort),
			"meta", "mark", "set", fwMark, "accept"); err != nil {
			return err
		}
	}
	if !cmdExists(ctx, "sh", "-c", "nft list ruleset | grep -Fq '"+udpRule+"'") {
		if err := runCmd(ctx, "nft", "add", "rule", "inet", nftTable, nftChain,
			"ip", "protocol", "udp", "tproxy", "to", fmt.Sprintf(":%d", tproxyPort),
			"meta", "mark", "set", fwMark, "accept"); err != nil {
			return err
		}
	}
	// 4) (опц.) системный DNS → локальный dns-in GUI
	if enableDNSRedirect {
		dnsRule := fmt.Sprintf("udp dport 53 redirect to :%d", dnsLocalPort)
		if !cmdExists(ctx, "sh", "-c", "nft list ruleset | grep -Fq '"+dnsRule+"'") {
			if err := runCmd(ctx, "nft", "add", "rule", "inet", nftTable, nftChain,
				"udp", "dport", "53", "redirect", "to", fmt.Sprintf(":%d", dnsLocalPort)); err != nil {
				return err
			}
		}
	}
	return nil
}

// CleanupTPROXY — снимает наши правила. Вызывать при Stop/Exit.
func CleanupTPROXY() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_ = runCmd(ctx, "nft", "delete", "table", "inet", nftTable)
	_ = runCmd(ctx, "ip", "rule", "del", "fwmark", fwMark, "lookup", rtTable)
	_ = runCmd(ctx, "ip", "route", "del", "local", "0.0.0.0/0", "dev", "lo", "table", rtTable)
	_ = runCmd(ctx, "ip", "-6", "rule", "del", "fwmark", fwMark, "lookup", rtTable)
	_ = runCmd(ctx, "ip", "-6", "route", "del", "local", "::/0", "dev", "lo", "table", rtTable)
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
		if status == service.StatusRunning {
			if dolog {
				fmt.Printf("Tunnel Service Already Running.\n")
			}
			return 0, "Tunnel Service Already Running."
		} else if status == service.StatusUnknown {
			s.Uninstall()
			s.Install()
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
