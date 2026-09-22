package cmd

import (
	"os"

	"github.com/sonar-probe/sonar/internal/features/auth"
	"github.com/spf13/cobra"
)

var Disable2FA = &cobra.Command{
	Use:   "disable-2fa",
	Short: "Force disable 2FA",
	Long:  `Force disable 2FA`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := auth.ForceDisableAllTwoFactor(); err != nil {
			cmd.Println("Error:", err)
			os.Exit(1)
		}
		cmd.Println("2FA has been disabled.")
		cmd.Println("Please restart the server to apply the changes.")
	},
}

func init() {
	RootCmd.AddCommand(Disable2FA)
}
