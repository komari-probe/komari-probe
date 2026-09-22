package cmd

import (
	"os"

	"github.com/sonar-probe/sonar/internal/platform/dbcore"
	"github.com/sonar-probe/sonar/internal/platform/settings"
	"github.com/sonar-probe/sonar/pkg/kv"
	"github.com/spf13/cobra"
)

var PermitPasswordLoginCmd = &cobra.Command{
	Use:   "permit-login",
	Short: "Force permit password login",
	Long:  `Force permit password login`,
	Run: func(cmd *cobra.Command, args []string) {
		dbcore.GetDBInstance()
		if err := kv.Set(settings.DisablePasswordLoginKey, false); err != nil {
			cmd.Println("Error:", err)
			os.Exit(1)
		}
		cmd.Println("Password login has been permitted.")
		cmd.Println("Please restart the server to apply the changes.")
	},
}

func init() {
	RootCmd.AddCommand(PermitPasswordLoginCmd)
}
