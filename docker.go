package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v2"
)

type DockerCompose struct {
	Services map[string]Service `yaml:"services"`
}

type Service struct {
	Image string `yaml:"image"`
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

func listServices(compose *DockerCompose, config *Config) []string {
	services := []string{}
	for serviceName := range compose.Services {
		if config != nil && !shouldProcessService(serviceName, config) {
			if verbose {
				log.Printf("Skipping service %s", serviceName)
			}
			continue
		}

		services = append(services, serviceName)
	}
	return services
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

func updateImages(config *Config) {
	if verbose {
		log.Println("Reading docker-compose file")
	}

	compose, err := readDockerCompose()
	if err != nil {
		log.Fatalf("Failed to read docker-compose file: %v", err)
	}

	services := listServices(compose, config)
	if len(services) == 0 {
		log.Println("No services to process based on the configuration.")
		return
	}

	log.Printf("Pulling latest images for %v services...", len(services))

	// Pull only the specified services
	args := []string{"compose", "pull"}
	args = append(args, services...)
	if err := runCommandAndLogOutput("docker", args...); err != nil {
		log.Printf("Error running 'docker compose pull %v': %v", services, err)
		return
	}

	log.Println("Comparing services for updates...")

	updateServices := false
	for _, serviceName := range services {
		service := compose.Services[serviceName]

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