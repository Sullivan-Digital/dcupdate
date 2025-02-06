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
	Sleep   int      `mapstructure:"sleep"`
}

var (
	verbose    bool
	configFile string
	config     *Config
	sleepTime  int
)

func initConfig() {
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("dcupdate")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Fatalf("Error reading config file: %v", err)
		}
	}

	var tmpConfig Config
	if err := viper.Unmarshal(&tmpConfig); err != nil {
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
			if sleepTime == 0 {
				sleepTime = config.Sleep
			}

			if daemon {
				for {
					updateImages(config)
					log.Printf("Sleeping for %d seconds...", sleepTime)
					time.Sleep(time.Duration(sleepTime) * time.Second)
				}
			} else {
				updateImages(config)
			}
		},
	}

	runCmd.Flags().Bool("daemon", false, "Run as a daemon")
	runCmd.Flags().IntVarP(&sleepTime, "sleep", "s", 0, "Set the sleep time in minutes for the daemon")

	rootCmd.AddCommand(runCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
