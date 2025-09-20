package v2

/*
#include "stdint.h"
*/
import (
	"context"
	"log"
	"net"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"

	"github.com/Darkmen203/rostovvpn-core/extension"
	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"

	"google.golang.org/grpc"
)

type HelloService struct {
	pb.UnimplementedHelloServer
}

type CoreService struct {
	pb.UnimplementedCoreServer

	ctx         context.Context
	cancel      context.CancelFunc
	instance    *box.Box
	urlHistory  adapter.URLTestHistoryStorage
	clashServer adapter.ClashServer
}

type TunnelService struct {
	pb.UnimplementedTunnelServiceServer
}

func StartGrpcServer(listenAddressG string, service string) (*grpc.Server, error) {
	lis, err := net.Listen("tcp", listenAddressG)
	if err != nil {
		log.Printf("failed to listen: %v", err)
		return nil, err
	}
	s := grpc.NewServer()
	if service == "core" {
		useFlutterBridge = false
		pb.RegisterCoreServer(s, &CoreService{})
		pb.RegisterExtensionHostServiceServer(s, &extension.ExtensionHostService{})
	} else if service == "hello" {
		pb.RegisterHelloServer(s, &HelloService{})
	} else if service == "tunnel" {
		pb.RegisterTunnelServiceServer(s, &TunnelService{})
	}
	log.Printf("Server listening on %s", listenAddressG)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Printf("failed to serve: %v", err)
		}
		log.Printf("Server stopped")
	}()
	return s, nil
}

func StartCoreGrpcServer(listenAddressG string) (*grpc.Server, error) {
	return StartGrpcServer(listenAddressG, "core")
}
func StartHelloGrpcServer(listenAddressG string) (*grpc.Server, error) {
	return StartGrpcServer(listenAddressG, "hello")
}
func StartTunnelGrpcServer(listenAddressG string) (*grpc.Server, error) {
	return StartGrpcServer(listenAddressG, "tunnel")
}
