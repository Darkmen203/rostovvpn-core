package config

import (
	context "context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"
	"github.com/sagernet/sing-box/option"
	dns "github.com/sagernet/sing-dns"
	grpc "google.golang.org/grpc"
)

const (
	serviceURL    = "http://localhost:18020"
	targetIP      = "127.0.0.1:18020"
	startEndpoint = "/start"
	stopEndpoint  = "/stop"
	// keep in sync with v2/tunnel_platform_service.go (service.Config.Name)
	macLaunchDaemonPath = "/Library/LaunchDaemons/RostovVPNTunnelService.plist"
)

var (
	tunnelServiceRunning   = false
	tunnelServiceInstalled = false
)

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
	res, err := stopTunnelRequest()
	tunnelServiceRunning = false
	cleanupTunnelServiceArtifacts()
	return res, err
}

func DeactivateTunnelService() (bool, error) {
	// if !isSupportedOS() {
	// 	return true, nil
	// }

	var (
		res bool
		err error
	)
	if tunnelServiceRunning {
		res, err = stopTunnelRequest()
	} else {
		go stopTunnelRequest()
		res = true
	}

	tunnelServiceRunning = false
	cleanupTunnelServiceArtifacts()
	return res, err
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

	return true, nil
}

func stopTunnelRequest() (bool, error) {
	ctx, cancelDial := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelDial()
	conn, err := grpc.DialContext(ctx, targetIP, grpc.WithInsecure(), grpc.WithBlock())
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
	if runtime.GOOS == "darwin" {
		tunnelServiceInstalled = true
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

func cleanupTunnelServiceArtifacts() {
	if runtime.GOOS != "darwin" {
		return
	}
	if !tunnelServiceInstalled && !macLaunchDaemonExists() {
		return
	}

	executablePath := getTunnelServicePath()
	if _, err := ExecuteCmd(executablePath, true, "tunnel", "uninstall"); err != nil {
		fmt.Printf("Tunnel service uninstall failed: %v\n", err)
	}
	tunnelServiceInstalled = false
}

func macLaunchDaemonExists() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	if _, err := os.Stat(macLaunchDaemonPath); err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	}
	return true
}
