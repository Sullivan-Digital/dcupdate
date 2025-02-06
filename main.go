package main

import (
	"fmt"
	"log"
	"net/http"
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

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		updateImages(config)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Images updated"))
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
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

	var updateCmd = &cobra.Command{
		Use:   "update",
		Short: "Update the images",
		Run: func(cmd *cobra.Command, args []string) {
			updateImages(config)
		},
	}

	var listenCmd = &cobra.Command{
		Use:   "listen [port]",
		Short: "Listen for update requests",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			listenPort := args[0]

			http.HandleFunc("/update", handleUpdate)
			log.Printf("Listening on port %s for update requests...", listenPort)
			if err := http.ListenAndServe(":"+listenPort, nil); err != nil {
				log.Fatalf("Failed to start HTTP server: %v", err)
			}
		},
	}

	var daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Run the updater",
		Run: func(cmd *cobra.Command, args []string) {
			if sleepTime == 0 {
				sleepTime = config.Sleep
			}

			for {
				updateImages(config)
				time.Sleep(time.Duration(sleepTime) * time.Second)
			}
		},
	}

	daemonCmd.Flags().IntVarP(&sleepTime, "sleep", "s", 0, "Set the sleep time in seconds for the daemon")

	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(listenCmd)
	rootCmd.AddCommand(daemonCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
