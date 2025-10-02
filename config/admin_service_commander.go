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
	"google.golang.org/grpc/credentials/insecure"
)

const (
	targetIP      = "127.0.0.1:18020"
)
const logDidNotConnect = "did not connect: %v"

var tunnelServiceRunning = false

func isSupportedOS() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "linux"
}

func isServiceListening() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	conn, err := grpc.DialContext(
		ctx,
		targetIP,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
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
	// сначала мягко
	_, _ = stopTunnelRequest()

	// затем жёстко по ОС
	switch runtime.GOOS {
	case "windows":
		// остановить сервис и добить зависший CLI
		_, _ = ExecuteCmd(getTunnelServicePath(), true, "tunnel", "stop")
		_, _ = ExecuteCmd("cmd.exe", true, "/C", "sc stop RostovVPNTunnelService")
		_, _ = ExecuteCmd("cmd.exe", true, "/C", "taskkill /IM RostovVPNCli.exe /T /F")
	case "darwin":
		_, _ = ExecuteCmd("bash", true, "-lc", "launchctl bootout system /Library/LaunchDaemons/com.rostovvpn.cli.plist 2>/dev/null || true")
		_, _ = ExecuteCmd("bash", true, "-lc", "launchctl bootout gui/$UID ~/Library/LaunchAgents/com.rostovvpn.cli.plist 2>/dev/null || true")
		_, _ = ExecuteCmd("bash", true, "-lc", "pkill -f RostovVPNCli || true")
	case "linux":
		_, _ = ExecuteCmd("bash", true, "-lc", "pkill -f RostovVPNCli || true")
		// если знаешь точное имя интерфейса — прибери:
		_, _ = ExecuteCmd("bash", true, "-lc", "ip link del rostovvpn0 2>/dev/null || true")
	}

	// подождать освобождения порта до 2 сек
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !isServiceListening() {
			return true, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	// даже если порт ещё занят — возвращаем false/err для корректного UX
	return !isServiceListening(), nil
}

func DeactivateTunnelService() (bool, error) {
	// мягко, best-effort
	ok, err := stopTunnelRequest()
	if err != nil {
		// не считаем это критической ошибкой — UI может потом сделать force
		return false, err
	}
	return ok, nil
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
	conn, err := grpc.DialContext(ctxDial, targetIP, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Printf(logDidNotConnect, err)
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
	ctxDial, cancelDial := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelDial()

	conn, err := grpc.DialContext(
		ctxDial,
		targetIP,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Printf(logDidNotConnect, err)
		return false, err
	}
	defer conn.Close()

	c := pb.NewTunnelServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = c.Stop(ctx, &pb.Empty{})
	if err != nil {
		// разовое повторение с новым контекстом
		ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel2()
		_, err2 := c.Stop(ctx2, &pb.Empty{})
		if err2 != nil {
			log.Printf("stop failed: %v / retry: %v", err, err2)
			return false, err2
		}
	}
	return true, nil
}

func ExitTunnelService() (bool, error) {
	ctxDial, cancelDial := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelDial()

	conn, err := grpc.DialContext(
		ctxDial,
		targetIP,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Printf(logDidNotConnect, err)
		return false, err
	}
	defer conn.Close()

	c := pb.NewTunnelServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = c.Exit(ctx, &pb.Empty{})
	if err != nil {
		log.Printf("exit failed: %v", err)
		return false, err
	}
	return true, nil
}

func runTunnelService(opt RostovVPNOptions) (bool, error) {
	executablePath := getTunnelServicePath()

	// 1) Пробуем установить сервис (UAC)
	if out, err := ExecuteCmd(executablePath, true, "tunnel", "install"); err != nil {
		// Если установка недоступна — пробуем фронтграундный режим сервиса
		out, err = ExecuteCmd(executablePath, true, "tunnel", "run")
		fmt.Println("Shell command executed (run):", out, err)
		if err != nil {
			return false, err
		}
	} else {
		fmt.Println("Shell command executed (install):", out, nil)
		// ВАЖНО: после install явно запускаем
		if _, err2 := ExecuteCmd(executablePath, true, "tunnel", "start"); err2 != nil {
			// если старт сервиса не удался — fallback на run
			out3, err3 := ExecuteCmd(executablePath, true, "tunnel", "run")
			fmt.Println("Shell command executed (start->run):", out3, err3)
			if err3 != nil {
				return false, err2
			}
		}
	}

	// 2) Ждём старта сервиса (учитываем UAC/launchd задержки)
	if err := waitForServiceReady(2 * time.Minute); err != nil {
		return false, err
	}

	// 3) Делаем gRPC Start
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
