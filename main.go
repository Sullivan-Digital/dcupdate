package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Config struct {
	Include []string    `mapstructure:"include"`
	Exclude []string    `mapstructure:"exclude"`
	Sleep   int         `mapstructure:"sleep"`
	Nexus   NexusConfig `mapstructure:"nexus"`
}

type NexusConfig struct {
	SecretKey string `mapstructure:"secret_key"`
}

var (
	verbose    bool
	configFile string
	config     *Config
	sleepTime  int

	updateMutex sync.Mutex
	updateCond  *sync.Cond
	updateFlag  bool
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

func verifyHMAC(body []byte, signature string, secretKey string) bool {
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedMAC)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	remoteAddr := r.RemoteAddr
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		log.Printf("Received request from %s (forwarded for %s) to %s", remoteAddr, forwardedFor, r.URL.String())
	} else {
		log.Printf("Received request from %s to %s", remoteAddr, r.URL.String())
	}
	if verbose {
		log.Printf("Request headers:")
		for name, values := range r.Header {
			for _, value := range values {
				log.Printf("  %s: %s", name, value)
			}
		}
	}

	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if config.Nexus.SecretKey != "" {
			log.Println("Verifying HMAC signature...")

			signature := r.Header.Get("X-Nexus-Webhook-Signature")
			if signature == "" {
				log.Println("Missing X-Nexus-Webhook-Signature header")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Remove whitespace from JSON body
			bodyStr := strings.ReplaceAll(string(body), " ", "")
			if !verifyHMAC([]byte(bodyStr), signature, config.Nexus.SecretKey) {
				log.Println("Invalid HMAC signature")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			log.Println("HMAC signature accepted")
		} else {
			log.Println("No secret key configured, skipping HMAC verification")
		}

		updateMutex.Lock()
		updateFlag = true
		updateCond.Signal()
		updateMutex.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Update request received"))
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func updateImagesLoop() {
	for {
		updateMutex.Lock()
		for !updateFlag {
			updateCond.Wait()
		}
		updateFlag = false
		updateMutex.Unlock()

		updateImages(config)
	}
}

func main() {
	updateCond = sync.NewCond(&updateMutex)

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
			go updateImagesLoop()
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
