package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Config struct {
	Include []string `mapstructure:"include"`
	Exclude []string `mapstructure:"exclude"`
}

var (
	verbose    bool
	configFile string
	config     *Config
)

func initConfig() {
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("dcupdate")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
	}

	var tmpConfig Config
	err := viper.Unmarshal(&tmpConfig)
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		return
	}

	if err != nil {
		log.Fatalf("unable to decode into config struct, %v", err)
	}

	config = &tmpConfig
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "dcupdate",
		Short: "Docker Compose Updater",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			initConfig()
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Specify the config file to use")

	var runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run the updater",
		Run: func(cmd *cobra.Command, args []string) {
			daemon, _ := cmd.Flags().GetBool("daemon")

			if daemon {
				for {
					updateImages(config)
					log.Println("Sleeping for 5 minutes...")
					time.Sleep(5 * time.Minute)
				}
			} else {
				updateImages(config)
			}
		},
	}

	runCmd.Flags().Bool("daemon", false, "Run as a daemon")

	rootCmd.AddCommand(runCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
