package v2

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Darkmen203/rostovvpn-core/config"
	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"
)

func (s *TunnelService) Start(ctx context.Context, in *pb.TunnelStartRequest) (*pb.TunnelResponse, error) {
	if in.ServerPort == 0 {
		in.ServerPort = 12334
	}
	useFlutterBridge = false
	res, err := Start(&pb.StartRequest{
		ConfigContent:          makeTunnelConfig(in.Ipv6, in.ServerPort, in.StrictRoute, in.EndpointIndependentNat, in.Stack),
		EnableOldCommandServer: false,
		DisableMemoryLimit:     true,
		EnableRawConfig:        true,
	})
	log.Printf("Start Result: %+v\n", res)
	if err != nil {
		return &pb.TunnelResponse{
			Message: err.Error(),
		}, err
	}
	return &pb.TunnelResponse{
		Message: "OK",
	}, err
}

func makeTunnelConfig(Ipv6 bool, ServerPort int32, StrictRoute bool, EndpointIndependentNat bool, Stack string) string {
	var ipv6Line string
	if Ipv6 {
		ipv6Line = "\t\t\t\"inet6_address\": \"fdfe:dcba:9876::1/126\",\n"
	}

	interfaceLine := ""
	if name := config.DefaultTunInterfaceName(); name != "" {
		interfaceLine = fmt.Sprintf("\t\t\t\"interface_name\": \"%s\",\n", name)
	}

	return fmt.Sprintf(`{
		"log":{
			"level": "warn"
		},
		"inbounds": [
		  {
			"type": "tun",
			"tag": "tun-in",
%s			"inet4_address": "172.19.0.1/30",
%s			"auto_route": true,
			"strict_route": %t,
			"endpoint_independent_nat": %t,
			"stack": "%s"
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
		  ],
		  "final": "socks-out"
		}
	  }`, interfaceLine, ipv6Line, StrictRoute, EndpointIndependentNat, Stack, ServerPort)
}
func (s *TunnelService) Stop(ctx context.Context, _ *pb.Empty) (*pb.TunnelResponse, error) {
	res, err := Stop()
	log.Printf("Stop Result: %+v\n", res)
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

