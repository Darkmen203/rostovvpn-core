package v2

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"

	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"
)

func (s *TunnelService) Start(ctx context.Context, in *pb.TunnelStartRequest) (*pb.TunnelResponse, error) {
	if in.ServerPort == 0 {
		in.ServerPort = 12334
	}
	useFlutterBridge = false
	// === прозрачно (tproxy) на Linux? ===
	transparent := (in.Transparent && runtime.GOOS == "linux")
	var cfg string
	if transparent {
		// defaults если не заданы
		if in.TproxyPort == 0 {
			in.TproxyPort = 12335
		}
		if in.LocalDnsPort == 0 {
			in.LocalDnsPort = 16450
		}
		cfg = makeTransparentConfig(in.ServerPort, in.TproxyPort) // tproxy-in -> socks-out(127.0.0.1:ServerPort)
		// Ставим policy routing + nft tproxy заранее (идемпотентно)
		if err := EnsureTPROXY(int(in.TproxyPort), int(in.LocalDnsPort), in.DnsRedirect); err != nil {
			log.Printf("[TunService] EnsureTPROXY failed: %v", err)
		}
	} else {
		cfg = makeTunnelConfig(in.Ipv6, in.ServerPort, in.StrictRoute, in.EndpointIndependentNat, in.Stack)
	}

	res, err := Start(&pb.StartRequest{
		ConfigContent:          cfg,
		EnableOldCommandServer: false,
		DisableMemoryLimit:     true,
		EnableRawConfig:        true,
		// (если у тебя тут дополнительные поля — оставь как было)
	})
	fmt.Printf("Start Result: %+v\n", res)
	if err != nil {
		// если не взлетело — аккуратно снимем правила
		if transparent {
			_ = CleanupTPROXY()
		}
		return &pb.TunnelResponse{
			Message: err.Error(),
		}, err
	}
	return &pb.TunnelResponse{
		Message: "OK",
	}, err
}

func makeTunnelConfig(Ipv6 bool, ServerPort int32, StrictRoute bool, EndpointIndependentNat bool, Stack string) string {
	var ipv6 string
	if Ipv6 {
		ipv6 = `      "inet6_address": "fdfe:dcba:9876::1/126",`
	} else {
		ipv6 = ""
	}
	base := `{
		"log":{
			"level": "warn"
		},
		"inbounds": [
		  {
			"type": "tun",
			"tag": "tun-in",
            "interface_name": "RostovVPNTunnel",
			"inet4_address": "172.19.0.1/30",
			` + ipv6 + `
			"auto_route": true,
			"strict_route": ` + fmt.Sprintf("%t", StrictRoute) + `,
			"endpoint_independent_nat": ` + fmt.Sprintf("%t", EndpointIndependentNat) + `,
			"stack": "` + Stack + `"
		  }
		],
		"outbounds": [
		  {
			"type": "socks",
			"tag": "socks-out",
			"server": "127.0.0.1",
			"server_port": ` + fmt.Sprintf("%d", ServerPort) + `,
			"version": "5"
		  },
		  {
			"type": "direct",
			"tag": "direct-out"
		  }
		],
		"route": {
		  "rules": [
			{
				"process_name":[
					"RostovVPN.exe",
					"RostovVPN",
					"RostovVPNCli",
					"RostovVPNCli.exe",
					"RostovVPN.exe",
					"RostovVPN",
					"RostovVPNCli",
					"RostovVPNCli.exe"
					],
				"outbound": "direct-out"
			}
		  ]
		}
	  }`

	return base
}

// Прозрачный (TPROXY) вариант для Linux: tproxy-in -> socks-out(127.0.0.1:ServerPort)
// Весь системный трафик перехватывается kernel rules и «приземляется» сюда,
// дальше уходит в GUI на mixed( ServerPort ).
func makeTransparentConfig(ServerPort, TproxyPort int32) string {
	return fmt.Sprintf(`{
  "log": { "level": "warn" },
  "inbounds": [
    {
      "type": "tproxy",
      "tag": "tproxy-in",
      "listen": "0.0.0.0",
      "listen_port": %d,
      "sniff": true
    }
  ],
  "outbounds": [
    {
      "type": "socks",
      "tag": "socks-out",
      "server": "127.0.0.1",
      "server_port": %d,
      "version": "5"
    },
    { "type": "direct", "tag": "direct-out" }
  ],
  "route": {
    "rules": [
      { "inbound": "tproxy-in", "outbound": "socks-out" },
      {
        "process_name":[ "RostovVPN.exe","RostovVPN","RostovVPNCli","RostovVPNCli.exe" ],
        "outbound": "direct-out"
      }
    ]
  }
}`, TproxyPort, ServerPort)
}

func (s *TunnelService) Stop(ctx context.Context, _ *pb.Empty) (*pb.TunnelResponse, error) {
	res, err := Stop()
	log.Printf("Stop Result: %+v\n", res)
	// На всякий случай снимаем TPROXY (если он был включен) — идемпотентно
	if runtime.GOOS == "linux" {
		_ = CleanupTPROXY()
	}
	if err != nil {
		return &pb.TunnelResponse{
			Message: err.Error(),
		}, err
	}

	return &pb.TunnelResponse{
		Message: "OK",
	}, err
}
func (s *TunnelService) Status(ctx context.Context, _ *pb.Empty) (*pb.TunnelResponse, error) {

	return &pb.TunnelResponse{
		Message: "Not Implemented",
	}, nil
}
func (s *TunnelService) Exit(ctx context.Context, _ *pb.Empty) (*pb.TunnelResponse, error) {
	Stop()
	os.Exit(0)
	return &pb.TunnelResponse{
		Message: "OK",
	}, nil
}
