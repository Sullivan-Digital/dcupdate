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
	verbose bool
	daemon  bool
)

type DockerCompose struct {
	Services map[string]Service `yaml:"services"`
}

type Service struct {
	Image string `yaml:"image"`
}

type ImageInspect struct {
	ID string `json:"Id"`
}

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&daemon, "daemon", false, "Run as a daemon")
	flag.Parse()
}

func readDockerCompose() (*DockerCompose, error) {
	var data []byte
	var err error

	if _, err = os.Stat("docker-compose.yml"); err == nil {
		data, err = os.ReadFile("docker-compose.yml")
	} else if _, err = os.Stat("docker-compose.yaml"); err == nil {
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

	log.Println("Pulling latest images...")

	if err := runCommandAndLogOutput("docker", "compose", "pull"); err != nil {
		log.Printf("Error running 'docker compose pull': %v", err)
		return
	}

	log.Println("Comparing services for updates..")

	updateServices := false
	for serviceName, service := range compose.Services {
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
