package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func getPodmanGateway() (string, error) {
	cmd := exec.Command("podman", "network", "inspect", "podman")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to inspect podman network: %v", err)
	}

	// Use regex to find the gateway IP
	re := regexp.MustCompile(`"gateway":\s*"([^"]+)"`)
	match := re.FindSubmatch(output)
	if match == nil {
		return "", fmt.Errorf("gateway IP not found in network configuration")
	}

	gatewayIP := string(match[1])
	return strings.TrimSpace(gatewayIP), nil
}

func getLocalIP() (string, error) {
	if os.Getenv("CONTAINER_OS") == "" {
		log.Fatal("CONTAINER_OS environment variable not set")
		return "", fmt.Errorf("CONTAINER_OS not set")
	}
	if os.Getenv("CONTAINER_OS") == "docker" {
		return "host.docker.internal", nil
	} else if os.Getenv("CONTAINER_OS") == "podman" {
		gateway, err := getPodmanGateway()
		if err != nil {
			return "", err
		}
		return gateway, nil
	} else {
		return "", fmt.Errorf("unsupported container OS: %s", os.Getenv("CONTAINER_OS"))
	}
}

func pullImage(container_os string, imagePath string) {
	// Pull the image
	// fmt.Printf("Pulling image: %s\n", imagePath)
	pullCmd := exec.Command(container_os, "pull", imagePath)
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		errMsg := fmt.Sprintf("failed to pull image: %v", err)
		log.Fatalf(errMsg)
	}
}

func stopContainer(container_os string, containerName string) {
	// Check if the container is running by looking for its name in the output
	psCmd := exec.Command(container_os, "ps", "-f", fmt.Sprintf("name=%s", containerName))
	output, err := psCmd.Output()
	if err != nil {
		errMsg := fmt.Sprintf("failed to check container status: %v", err)
		log.Fatalf(errMsg)
	}

	if strings.Contains(string(output), containerName) {
		fmt.Printf("Stopping container: %s\n", containerName)
		stopCmd := exec.Command(container_os, "stop", containerName)
		if err := stopCmd.Run(); err != nil {
			errMsg := fmt.Sprintf("failed to stop container: %v", err)
			log.Fatalf(errMsg)
		}

		rmCmd := exec.Command(container_os, "rm", containerName)
		if err := rmCmd.Run(); err != nil {
			errMsg := fmt.Sprintf("failed to remove container: %v", err)
			log.Fatalf(errMsg)
		}
		fmt.Println("Container stopped and removed successfully")
	} else {
		fmt.Printf("Container %s is not running\n", containerName)
	}
}
