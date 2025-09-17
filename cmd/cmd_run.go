package cmd

import (
	v2 "github.com/Darkmen203/rostovvpn-core/v2"

	"github.com/spf13/cobra"
)

var commandRun = &cobra.Command{
	Use:   "run",
	Short: "run",
	Args:  cobra.OnlyValidArgs,
	Run:   runCommand,
}

func init() {
	// commandRun.PersistentFlags().BoolP("help", "", false, "help for this command")
// commandRun.Flags().StringVarP(&rostovVPNSettingPath, "rostovVPN", "d", "", "RostovVPN Setting JSON Path")

	addHConfigFlags(commandRun)

	mainCommand.AddCommand(commandRun)
}

func runCommand(cmd *cobra.Command, args []string) {
	v2.Setup("./tmp", "./", "./tmp", 0, false)
	v2.RunStandalone(rostovVPNSettingPath, configPath, defaultConfigs)
}
