package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func validateValue(value string, validValues []string) (bool, error) {
	isValidType := false
	for _, validType := range validValues {
		if value == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		log.Fatalf("ERROR: Invalid persistence value '%s'. Must be one of %v", value, validValues)
	}
	return isValidType, nil
}

func combineMaps(mapOne, mapTwo map[string]string) map[string]string {
	secretMap := make(map[string]string)
	for k, v := range mapOne {
		secretMap[k] = v
	}
	for k, v := range mapTwo {
		secretMap[k] = v
	}
	return secretMap
}

func replaceEnvVariables(input string, secretMap map[string]string) (string, error) {
	re := regexp.MustCompile(`env\.(\w+)`)
	result := re.ReplaceAllStringFunc(input, func(match string) string {
		key := strings.TrimPrefix(match, "env.")
		if value, ok := secretMap[key]; ok {
			return value
		}
		return match
	})

	if strings.Contains(result, "env.") {
		return "", fmt.Errorf("unresolved env variables remain in: %s", result)
	}

	return result, nil
}

func main() {
	if os.Getenv("CONTAINER_OS") == "" {
		log.Fatal("ERROR: CONTAINER_OS environment variable not set!")
	}

	container_os := os.Getenv("CONTAINER_OS")
	container_os_types := []string{"docker", "podman"}
	valid, err := validateValue(container_os, container_os_types)
	if !valid {
		log.Fatalf("ERROR: Invalid CONTAINER_OS value '%s'. Must be one of %v!", container_os, container_os_types)
	}

	var env, service string

	// Define flags
	flag.StringVar(&env, "env", "", "Environment (dev/beta/prod/etc)")
	flag.StringVar(&service, "service", "", "Name of service to run")

	// Parse flags
	flag.Parse()

	// Verify arguments are set
	if env == "" || service == "" {
		fmt.Fprintf(os.Stderr, "USAGE: %s --env <env> --service <service>\n", os.Args[0])
		os.Exit(1)
	}

	// Use env and service variables as needed
	fmt.Printf("Restarting %s %s\n", service, env)
	serviceDefaultName := fmt.Sprintf("var/%s/default", service)
	serviceEnvName := fmt.Sprintf("var/%s/%s", service, env)

	defaultMap := returnSecrets("var/global/default")
	serviceDefault := returnSecrets(serviceDefaultName)
	serviceEnv := returnSecrets(serviceEnvName)
	defaultsMap := combineMaps(defaultMap, serviceDefault)
	serviceMap := combineMaps(defaultsMap, serviceEnv)

	requiredEntries := []string{"container_image_tag_param", "container_base_image"}
	for _, entry := range requiredEntries {
		if _, exists := serviceMap[entry]; !exists {
			log.Fatalf("ERROR: Required entry '%s' not found in service secrets\n", entry)
		}
	}

	persistence := "service"
	if _, exists := serviceMap["persistence"]; exists {
		persistenceTypes := []string{"job", "service"}
		persistenceValue := serviceMap["persistence"]
		valid, err := validateValue(persistenceValue, persistenceTypes)
		if !valid {
			log.Fatalf("ERROR: Invalid persistence value '%s'. Must be one of %v. Error: %v", persistenceValue, persistenceTypes, err)
		}
	}

	var container_persistence string
	if persistence == "service" {
		container_persistence = "--restart unless-stopped"
	} else {
		log.Fatalf("ERROR: Invalid or unconfiguredpersistence value '%s'.", persistence)
	}

	// if not PGDATABASE, use PGUSER
	pgDatabase, ok := serviceMap["PGDATABASE"]
	if !ok {
		fmt.Printf("WARNING: PGDATABASE not found in service map, using %s\n", serviceMap["PGUSER"])
		pgDatabase = serviceMap["PGUSER"]
	}

	// if not container_service_name_prefix, use service
	serviceNamePrefix, ok := serviceMap["container_service_name_prefix"]
	if !ok {
		fmt.Printf("WARNING: container_service_name_prefix not found in service map, using %s\n", service)
		serviceNamePrefix = service
	}
	serviceName := fmt.Sprintf("%s-%s", serviceNamePrefix, env)

	// get IP for communicating with host services, also use for PGHOST
	localIP, err := getLocalIP()
	if err != nil {
		log.Fatalf("Error getting local IP: %v", err)
	}
	serviceMap["PGHOST"] = localIP

	// Set new environment variables
	os.Setenv("PGUSER", serviceMap["PGUSER"])
	os.Setenv("PGPASSWORD", serviceMap["PGPASSWORD"])
	os.Setenv("PGDATABASE", pgDatabase)

	// Print the results (for demonstration)
	imageTag, err := getImageTag(serviceMap["container_image_tag_param"])
	if err != nil {
		log.Fatalf("Error getting image tag: %v", err)
	}
	container_image := fmt.Sprintf("%s:%s", serviceMap["container_base_image"], imageTag)

	// Create a string builder for environment variables
	var envVars strings.Builder
	for key, value := range serviceMap {
		if !strings.HasPrefix(key, "container_") {
			envVars.WriteString(fmt.Sprintf("-e %s=\"%s\" ", strings.ToUpper(key), value))
		}
	}
	// Trim the trailing space
	envString := strings.TrimSpace(envVars.String())

	replacedEnvString, err := replaceEnvVariables(envString, serviceMap)
	if err != nil {
		log.Fatalf("Error replacing environment variables: %v", err)
	}
	container_cmd_string := fmt.Sprintf("%s run -d %s --name %s %s %s", container_os, container_persistence, serviceName, replacedEnvString, container_image)

	if command, exists := serviceMap["container_command"]; exists {
		config, err := replaceEnvVariables(command, serviceMap)
		if err != nil {
			log.Fatalf("Error replacing environment variables: %v", err)
		}
		container_cmd_string = fmt.Sprintf("%s %s", container_cmd_string, config)
	}

	pullImage(container_os, container_image)
	stopContainer(container_os, serviceName)
	// Split the command string into command and arguments
	cmdParts := strings.Fields(container_cmd_string)
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

	// Capture the command output
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Error executing command: %v\nOutput: %s", err, output)
	}

	fmt.Printf("Command executed successfully. Output:\n%s\n", output)
}
