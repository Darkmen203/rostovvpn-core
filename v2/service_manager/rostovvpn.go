package service_manager

import (
	"fmt"

	"github.com/sagernet/sing-box/adapter"
)

var (
	services    = []adapter.Service{}
	preservices = []adapter.Service{}
)

func RegisterPreservice(service adapter.Service) {
	preservices = append(preservices, service)
}

func Register(service adapter.Service) {
	services = append(services, service)
}

func StartServices() error {
	fmt.Print("[StartServices in service_manager/rostovvpn.go] !!! ", " !!! [StartServices in service_manager/rostovvpn.go]")
	CloseServices()
	for _, stage := range adapter.ListStartStages {
		fmt.Print("[StartServices in service_manager/rostovvpn.go] !!! ", stage, " !!! [StartServices in service_manager/rostovvpn.go]")

		for _, service := range preservices {
			if err := adapter.LegacyStart(service, stage); err != nil {
				return err
			}
		}
		for _, service := range services {
			if err := adapter.LegacyStart(service, stage); err != nil {
				return err
			}
		}
	}
	return nil
}

func CloseServices() error {
	for _, service := range services {
		if err := service.Close(); err != nil {
			return err
		}
	}
	for _, service := range preservices {
		if err := service.Close(); err != nil {
			return err
		}
	}
	return nil
}
