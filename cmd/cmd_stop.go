package cmd

import (
	v2 "github.com/Darkmen203/rostovvpn-core/v2"

	"github.com/spf13/cobra"
)

var commandStop = &cobra.Command{
	Use:   "stop",
	Short: "stop",
	Args:  cobra.OnlyValidArgs,
	Run:   stopCommand,
}

func init() {
	// commandRun.PersistentFlags().BoolP("help", "", false, "help for this command")
// commandRun.Flags().StringVarP(&rostovVPNSettingPath, "rostovVPN", "d", "", "RostovVPN Setting JSON Path")
	mainCommand.AddCommand(commandStop)
}

func stopCommand(cmd *cobra.Command, args []string) {
	v2.Stop()
}
