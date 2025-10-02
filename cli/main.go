package main

import (
	"os"

	"github.com/Darkmen203/rostovvpn-core/cli/cmdroot"
	"github.com/Darkmen203/rostovvpn-core/cmd"
)

type UpdateRequest struct {
	Description     string `json:"description,omitempty"`
	PrivatePods     bool   `json:"private_pods"`
	OperatingMode   string `json:"operating_mode,omitempty"`
	ActivationState string `json:"activation_state,omitempty"`
}

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "tunnel" {
		// Новые команды: tunnel run/start/stop/install/uninstall/exit/deactivate-force
		// Возврат с кодом выхода
		cmdroot.Main()
		return
	}
	// Старое поведение остаётся без изменений
	cmd.ParseCli(args)
}