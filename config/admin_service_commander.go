package config

import (
	bytes "bytes"
	context "context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"
	"github.com/sagernet/sing-box/option"
	dns "github.com/sagernet/sing-dns"
	grpc "google.golang.org/grpc"
)

const (
	nftTable = "rostovvpn"
	nftChain = "prerouting"
	fwMark   = "0x1"
	rtTable  = "100"
	// Примечание: таблица 100 — пользовательская, только для нашего fwmark.
)

const (
	serviceURL    = "http://localhost:18020"
	targetIP      = "127.0.0.1:18020"
	startEndpoint = "/start"
	stopEndpoint  = "/stop"
)

var tunnelServiceRunning = false

func isSupportedOS() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "linux"
}

// Быстрый тест «жив ли сервис»: пробуем короткий gRPC dial.
func isServiceListening() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	conn, err := grpc.DialContext(ctx, targetIP, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitForServiceReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isServiceListening() {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("service did not become ready on %s within %s", targetIP, timeout)
}

func ActivateTunnelService(opt RostovVPNOptions) (bool, error) {
	tunnelServiceRunning = true
	// if !isSupportedOS() {
	// 	return false, E.New("Unsupported OS: " + runtime.GOOS)
	// }

	go startTunnelRequestWithFailover(opt, true)
	return true, nil
}

func DeactivateTunnelServiceForce() (bool, error) {
	return stopTunnelRequest()
}

func DeactivateTunnelService() (bool, error) {
	// if !isSupportedOS() {
	// 	return true, nil
	// }

	if tunnelServiceRunning {
		res, err := stopTunnelRequest()
		if err != nil {
			tunnelServiceRunning = false
		}
		return res, err
	} else {
		go stopTunnelRequest()
	}

	return true, nil
}

func startTunnelRequestWithFailover(opt RostovVPNOptions, installService bool) {
	if ok, _ := startTunnelRequest(opt, installService); !ok {
		// Если упёрлись в «service is not running» — значит сервис ещё не успел подняться.
		if err := waitForServiceReady(2 * time.Minute); err != nil {
			fmt.Printf("Start Tunnel Failed: %v\n", err)
			return
		}
		// Повторяем запуск несколько раз с бэкоффом
		for attempt := 0; attempt < 5; attempt++ {
			if ok, err := startTunnelRequest(opt, false); ok && err == nil {
				return
			}
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
		fmt.Println("Start Tunnel Failed after service became ready")
	}
}

func startTunnelRequest(opt RostovVPNOptions, installService bool) (bool, error) {
	// Если сервис не отвечает — устанавливаем/запускаем его сразу (ранний UAC)
	if !isServiceListening() {
		if installService {
			return runTunnelService(opt)
		}
		return false, fmt.Errorf("service is not running")
	}

	ctxDial, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelDial()
	conn, err := grpc.DialContext(ctxDial, targetIP, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Printf("did not connect: %v", err)
		if installService {
			// Попробуем перезапустить сервис, если внезапно перестал отвечать
			_, _ = ExitTunnelService()
			return runTunnelService(opt)
		}
		return false, err
	}
	defer conn.Close()
	c := pb.NewTunnelServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	// Останавливаем молча: если не работало — не страшно
	_, _ = c.Stop(ctx, &pb.Empty{})
	res, err := c.Start(ctx, &pb.TunnelStartRequest{
		Ipv6:                   opt.IPv6Mode == option.DomainStrategy(dns.DomainStrategyUseIPv6),
		ServerPort:             int32(opt.InboundOptions.MixedPort),
		StrictRoute:            opt.InboundOptions.StrictRoute,
		EndpointIndependentNat: true,
		Stack:                  opt.InboundOptions.TUNStack,
	})
	if err != nil {
		log.Printf("could not greet: %+v %+v", res, err)

		if installService {
			ExitTunnelService()
			return runTunnelService(opt)
		}
		return false, err
	}

	// Успешный Start — включаем прозрачный перехват на Linux (идемпотентно)
	if runtime.GOOS == "linux" {
		// Берём tproxy/dns порты из опций ядра (см. твои RostovVPNOptions)
		// opt.TProxyPort / opt.LocalDnsPort — это те же значения, что в конфиге ("tproxy-port", "local-dns-port")
		// Если у тебя они лежат в другом месте — подставь нужные поля.
		tp := int(opt.TProxyPort)
		if tp == 0 {
			tp = 12335
		}
		ldns := int(opt.LocalDnsPort)
		if ldns == 0 {
			ldns = 16450
		}
		_ = EnsureTPROXY(tp, ldns, true)
	}

	return true, nil
}

func stopTunnelRequest() (bool, error) {
	ctx, cancelDial := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelDial()
	conn, err := grpc.DialContext(ctx, targetIP, grpc.WithInsecure(), grpc.WithBlock())
	// В любом случае попытаться снять наши сетевые правила (Linux)
	defer CleanupTPROXY()

	if err != nil {
		log.Printf("did not connect: %v", err)
		return false, err
	}
	defer conn.Close()
	c := pb.NewTunnelServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.Stop(ctx, &pb.Empty{})
	if err != nil {
		log.Printf("did not Stopped: %v %v", res, err)
		_, _ = c.Stop(ctx, &pb.Empty{})
		return false, err
	}

	return true, nil
}

func ExitTunnelService() (bool, error) {
	ctx, cancelDial := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelDial()
	conn, err := grpc.DialContext(ctx, targetIP, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Printf("did not connect: %v", err)
		return false, err
	}
	defer conn.Close()
	c := pb.NewTunnelServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()

	// best-effort: снять tproxy/policy routing
	_ = CleanupTPROXY()

	res, err := c.Exit(ctx, &pb.Empty{})
	if res != nil {
		log.Printf("did not exit: %v %v", res, err)
		return false, err
	}

	return true, nil
}

func runTunnelService(opt RostovVPNOptions) (bool, error) {
	executablePath := getTunnelServicePath()
	// 1) Пытаемся установить сервис с повышением привилегий
	out, err := ExecuteCmd(executablePath, true, "tunnel", "install")
	if err != nil {
		// если установка недоступна, пытаемся запустить как «run» (также под UAC)
		out, err = ExecuteCmd(executablePath, true, "tunnel", "run")
		fmt.Println("Shell command executed (run):", out, err)
		if err != nil {
			return false, err
		}
	}
	// 2) Ждём старта сервиса (учитываем, что юзер может долго жать UAC)
	if err := waitForServiceReady(2 * time.Minute); err != nil {
		return false, err
	}
	// 3) Когда сервис доступен, делаем Start
	return startTunnelRequest(opt, false)
}

func getTunnelServicePath() string {
	var fullPath string
	exePath, _ := os.Executable()
	binFolder := filepath.Dir(exePath)
	switch runtime.GOOS {
	case "windows":
		fullPath = "RostovVPNCli.exe"
	case "darwin":
		fallthrough
	default:
		fullPath = "RostovVPNCli"
	}

	abspath, _ := filepath.Abs(filepath.Join(binFolder, fullPath))
	return abspath
}

// ===== Linux TPROXY helper'ы (идемпотентные) =====
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
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Run() == nil
}

// EnsureTPROXY — включает policy routing + nft tproxy (Linux). Безопасно вызывать повторно.
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
	// IPv6: best-effort (не критично, если упадёт)
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
	// 4) (опционально) перехват системного DNS на локальный dns-in
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

// CleanupTPROXY — снимает наши правила (идемпотентно).
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
