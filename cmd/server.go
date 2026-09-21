package cmd

import (
	"github.com/komari-monitor/komari/internal/app"

	"github.com/spf13/cobra"
)

// listen holds the -l/--listen flag value. It's only read after cobra
// populates it in Run, so it doesn't need to live in a shared package.
var listen string

var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the server",
	Long:  `Start the server`,
	Run: func(cmd *cobra.Command, args []string) {
		app.Start(listen)
	},
}

func init() {
	// 从环境变量获取监听地址
	listenAddr := GetEnv("KOMARI_LISTEN", "0.0.0.0:25774")
	ServerCmd.PersistentFlags().StringVarP(&listen, "listen", "l", listenAddr, "监听地址 [env: KOMARI_LISTEN]")
	RootCmd.AddCommand(ServerCmd)
}
