package cmd

import (
	"fmt"
	"time"

	"github.com/Darkmen203/rostovvpn-core/config"
	v2 "github.com/Darkmen203/rostovvpn-core/v2"

	"github.com/spf13/cobra"
)

var commandService = &cobra.Command{
	Use:       "tunnel run/start/stop/install/uninstall/activate/deactivate/exit",
	Short:     "Tunnel Service run/start/stop/install/uninstall/activate/deactivate/exit",
	ValidArgs: []string{"run", "start", "stop", "install", "uninstall", "activate", "deactivate", "exit"},
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		arg := args[0]
		switch arg {
		case "activate":
			config.ActivateTunnelService(config.HiddifyOptions{
				InboundOptions: config.InboundOptions{
					EnableTunService: true,
					MixedPort:        12334,
					TUNStack:         "gvisor",
				},
			})
			<-time.After(1 * time.Second)

		case "deactivate":
			config.DeactivateTunnelServiceForce()
		case "exit":
			config.ExitTunnelService()
		default:
			code, out := v2.StartTunnelService(arg)
			fmt.Printf("exitCode:%d msg=%s", code, out)
		}
	},
}
