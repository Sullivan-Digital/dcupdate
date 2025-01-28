package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

var (
	verbose    bool
	daemon     bool
	configFile string
)

type DockerCompose struct {
	Services map[string]Service `yaml:"services"`
}

type Service struct {
	Image string `yaml:"image"`
}

type Config struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&daemon, "daemon", false, "Run as a daemon")
	flag.StringVar(&configFile, "config", "", "Specify the config file to use")
	flag.Parse()
}

func readOneOf(files ...string) ([]byte, error) {
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			return os.ReadFile(file)
		}
	}

	return nil, os.ErrNotExist
}

func readDockerCompose() (*DockerCompose, error) {
	data, err := readOneOf("docker-compose.yml", "docker-compose.yaml")
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("no docker-compose file found")
	}

	if err != nil {
		return nil, err
	}

	var compose DockerCompose
	err = yaml.Unmarshal(data, &compose)
	if err != nil {
		return nil, err
	}

	return &compose, nil
}

func readConfig() (*Config, error) {
	var data []byte
	var err error

	if configFile != "" {
		data, err = os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read specified config file: %v", err)
		}
	} else {
		data, err = readOneOf("docker-compose-updater.yml", "docker-compose-updater.yaml")
		if os.IsNotExist(err) {
			return nil, nil
		}
	}

	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func shouldProcessService(serviceName string, config *Config) bool {
	if len(config.Exclude) > 0 {
		for _, exclude := range config.Exclude {
			if serviceName == exclude {
				return false
			}
		}
	}

	if len(config.Include) > 0 {
		for _, include := range config.Include {
			if serviceName == include {
				return true
			}
		}

		return false
	}

	return true
}

func getImageDigest(image string) (string, error) {
	cmd := exec.Command("docker", "image", "inspect", "--format={{.Id}}", image)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if verbose {
		log.Printf("Output of 'docker image inspect %s':\n%s", image, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func getContainerDigest(containerName string) (string, error) {
	cmd := exec.Command("docker", "container", "inspect", "--format={{.Image}}", containerName)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if verbose {
		log.Printf("Output of 'docker container inspect %s':\n%s", containerName, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func getContainerName(serviceName string) (string, error) {
	cmd := exec.Command("docker", "compose", "ps", "--format", "{{.Name}}", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if verbose {
		log.Printf("Output of 'docker compose ps %s':\n%s", serviceName, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func runCommandAndLogOutput(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if verbose {
				log.Printf("[stdout] %s", scanner.Text())
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if verbose {
				log.Printf("[stderr] %s", scanner.Text())
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	return nil
}

func updateImages() {
	if verbose {
		log.Println("Reading docker-compose file")
	}

	compose, err := readDockerCompose()
	if err != nil {
		log.Fatalf("Failed to read docker-compose file: %v", err)
	}

	config, err := readConfig()
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	log.Println("Pulling latest images...")

	if err := runCommandAndLogOutput("docker", "compose", "pull"); err != nil {
		log.Printf("Error running 'docker compose pull': %v", err)
		return
	}

	log.Println("Comparing services for updates..")

	updateServices := false
	for serviceName, service := range compose.Services {
		if config != nil && !shouldProcessService(serviceName, config) {
			if verbose {
				log.Printf("Skipping service %s", serviceName)
			}
			continue
		}

		updateThisService := false
		if verbose {
			log.Printf("Checking service %s with image %s", serviceName, service.Image)
		}

		latestDigest, err := getImageDigest(service.Image)
		if err != nil {
			log.Printf("Failed to get latest image digest for %s: %v", service.Image, err)
			continue
		}

		containerName, err := getContainerName(serviceName)
		if err != nil {
			log.Printf("Failed to get container name for service %s: %v", serviceName, err)
			continue
		}

		currentDigest, err := getContainerDigest(containerName)
		if err != nil {
			log.Printf("Failed to get current image digest for container %s: %v", containerName, err)
			continue
		}

		if verbose {
			log.Printf("Latest image digest for %s: %s", service.Image, latestDigest)
			log.Printf("Current image digest for %s: %s", containerName, currentDigest)
		}

		if currentDigest != latestDigest {
			updateServices = true
			updateThisService = true
		}

		if updateThisService {
			log.Printf("%s: (!) update required", serviceName)
		} else {
			log.Printf("%s: up to date", serviceName)
		}
	}

	if updateServices {
		log.Println("Updating all services...")

		if err := runCommandAndLogOutput("docker", "compose", "down"); err != nil {
			log.Printf("Error running 'docker compose down': %v", err)
		}

		if err := runCommandAndLogOutput("docker", "compose", "up", "-d"); err != nil {
			log.Printf("Error running 'docker compose up -d': %v", err)
		}
	}

	log.Println("Done")
}

func main() {
	if daemon {
		for {
			updateImages()
			log.Println("Sleeping for 5 minutes...")
			time.Sleep(5 * time.Minute)
		}
	} else {
		updateImages()
	}
}
