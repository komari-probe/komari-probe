package cmd

import (
	"fmt"
	"os"

	"github.com/sonar-probe/sonar/internal/platform/dbcore"

	"github.com/spf13/cobra"
)

func GetEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

var RootCmd = &cobra.Command{
	Use:   "sonar",
	Short: "Sonar is a simple, lightweight server monitoring tool",
	Long:  `Sonar is a simple, lightweight server monitoring tool.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.SetArgs([]string{"server"})
		cmd.Execute()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&dbcore.DatabaseType, "db-type", "t", "sqlite", "Database type (sqlite)")
	RootCmd.PersistentFlags().StringVarP(&dbcore.DatabaseFile, "database", "d", "./data/sonar.db", "SQLite database file path")
}
