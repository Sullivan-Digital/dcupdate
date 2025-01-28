package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"gopkg.in/yaml.v2"
)

var verbose bool

type DockerCompose struct {
	Services map[string]Service `yaml:"services"`
}

type Service struct {
	Image string `yaml:"image"`
}

// Define a struct to match the JSON structure
type Manifest struct {
	Config struct {
		Digest string `json:"digest"`
	} `json:"config"`
}

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()
}

func readDockerCompose() (*DockerCompose, error) {
	var data []byte
	var err error

	// Try reading docker-compose.yml
	if _, err = os.Stat("docker-compose.yml"); err == nil {
		data, err = os.ReadFile("docker-compose.yml")
	} else if _, err = os.Stat("docker-compose.yaml"); err == nil {
		// If docker-compose.yml doesn't exist, try docker-compose.yaml
		data, err = os.ReadFile("docker-compose.yaml")
	} else {
		return nil, fmt.Errorf("no docker-compose.yml or docker-compose.yaml file found")
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

func getCurrentImageHash(image string) (string, error) {
	cmd := exec.Command("docker", "image", "inspect", "--format={{.Id}}", image)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if verbose {
		log.Printf("Output of 'docker image inspect %s':\n%s", image, string(output))
	}
	return string(output), nil
}

func getLatestImageHash(image string) (string, error) {
	cmd := exec.Command("docker", "manifest", "inspect", image)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if verbose {
		log.Printf("Output of 'docker manifest inspect %s':\n%s", image, string(output))
	}

	var manifest Manifest
	err = json.Unmarshal(output, &manifest)
	if err != nil {
		return "", err
	}

	return manifest.Config.Digest, nil
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

	// Log stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if verbose {
				log.Printf("[stdout] %s", scanner.Text())
			}
		}
	}()

	// Log stderr
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

	if verbose {
		log.Println("Checking for updates")
	}
	updateServices := true
	for serviceName, service := range compose.Services {
		if verbose {
			log.Printf("Checking service %s with image %s", serviceName, service.Image)
		}

		currentHash, err := getCurrentImageHash(service.Image)
		if err != nil {
			log.Printf("Failed to get current image hash for %s: %v", service.Image, err)
			continue
		}

		if verbose {
			log.Printf("Current image hash for %s: %s", service.Image, currentHash)
		}

		latestHash, err := getLatestImageHash(service.Image)
		if err != nil {
			log.Printf("Failed to get latest image hash for %s: %v", service.Image, err)
			continue
		}

		if verbose {
			log.Printf("Latest image hash for %s: %s", service.Image, latestHash)
		}

		if currentHash != latestHash {
			updateServices = true
			log.Printf("New image available for %s, will update.", serviceName)
			break
		}
	}

	if updateServices {
		log.Println("Updating all services")

		// Run docker compose down
		if err := runCommandAndLogOutput("docker", "compose", "down"); err != nil {
			log.Printf("Error running 'docker compose down': %v", err)
		}

		// Run docker compose pull
		if err := runCommandAndLogOutput("docker", "compose", "pull"); err != nil {
			log.Printf("Error running 'docker compose pull': %v", err)
		}

		// Run docker compose up -d
		if err := runCommandAndLogOutput("docker", "compose", "up", "-d"); err != nil {
			log.Printf("Error running 'docker compose up -d': %v", err)
		}
	}
}

func main() {
	for {
		updateImages()
		time.Sleep(5 * time.Minute)
	}
}
