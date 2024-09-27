package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

func getAWSConfig() (aws.Config, error) {
	opts := []func(*config.LoadOptions) error{}

	region := os.Getenv("AWS_DEFAULT_REGION")
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	return config.LoadDefaultConfig(context.TODO(), opts...)
}

func getImageTag(paramName string) (string, error) {
	// Load the AWS SDK configuration
	cfg, err := getAWSConfig()
	if err != nil {
		return "", fmt.Errorf("unable to load SDK config: %v", err)
	}

	// Create an SSM client
	client := ssm.NewFromConfig(cfg)

	// Get the parameter
	input := &ssm.GetParameterInput{
		Name: &paramName,
	}

	result, err := client.GetParameter(context.TODO(), input)
	if err != nil {
		return "", fmt.Errorf("unable to get parameter: %v", err)
	}

	// Return the parameter value
	return *result.Parameter.Value, nil
}

func returnSecrets(secretNames ...string) map[string]string {
	// Create AWS session
	cfg, err := getAWSConfig()
	if err != nil {
		log.Fatalf("Error creating session: %v", err)
	}

	// Create Secrets Manager client
	svc := secretsmanager.NewFromConfig(cfg)

	secretMap := make(map[string]string)

	for _, secretName := range secretNames {
		// Get secret value
		input := &secretsmanager.GetSecretValueInput{
			SecretId: aws.String(secretName),
		}
		result, err := svc.GetSecretValue(context.Background(), input)
		if err != nil {
			log.Fatalf("Error getting secret value for %s: %v\n", secretName, err)
		}

		// Parse secret JSON
		var tempMap map[string]string
		err = json.Unmarshal([]byte(*result.SecretString), &tempMap)
		if err != nil {
			log.Fatalf("Error parsing secret JSON for %s: %v\n", secretName, err)
		}

		// Merge tempMap into secretMap
		for k, v := range tempMap {
			secretMap[k] = v
		}
	}

	return secretMap
}

func printContainerMapPorts() error {
	// Create AWS session
	cfg, err := getAWSConfig()
	if err != nil {
		return fmt.Errorf("error creating session: %v", err)
	}

	// Create Secrets Manager client
	svc := secretsmanager.NewFromConfig(cfg)

	// List all secrets
	input := &secretsmanager.ListSecretsInput{}
	_, err = svc.ListSecrets(context.TODO(), input)
	if err != nil {
		return err
	}

	portMap := make(map[string]string)

	// Handle pagination
	var nextToken *string
	for {
		input := &secretsmanager.ListSecretsInput{
			NextToken: nextToken,
		}
		output, err := svc.ListSecrets(context.TODO(), input)
		if err != nil {
			return fmt.Errorf("error listing secrets: %v", err)
		}

		// Process secrets in this page
		for _, secret := range output.SecretList {
			name := *secret.Name
			if strings.HasPrefix(name, "var/") {
				parts := strings.Split(name, "/")
				if len(parts) == 3 {
					service := parts[1]
					env := parts[2]

					// Get secret value
					getSecretInput := &secretsmanager.GetSecretValueInput{
						SecretId: aws.String(name),
					}
					secretResult, err := svc.GetSecretValue(context.TODO(), getSecretInput)
					if err != nil {
						log.Printf("Error getting secret value for %s: %v", name, err)
						continue // Skip this secret and continue with the next one
					}

					// Parse secret JSON
					var secretMap map[string]string
					err = json.Unmarshal([]byte(*secretResult.SecretString), &secretMap)
					if err != nil {
						log.Printf("Error parsing secret JSON for %s: %v", name, err)
						continue // Skip this secret and continue with the next one
					}

					// Extract container_map_port
					if port, ok := secretMap["container_map_port"]; ok {
						key := fmt.Sprintf("%s-%s", service, env)
						portMap[key] = port
						// } else {
						// 	log.Printf("Secret %s does not contain 'container_map_port' key", name)
					}
					// } else {
					// 	log.Printf("Secret %s does not match expected format", name)
				}
			}
		}

		nextToken = output.NextToken
		if nextToken == nil {
			break
		}
	}

	// Create a slice of structs to hold the data
	type portInfo struct {
		key  string
		port int
	}
	var portInfos []portInfo

	// Convert port strings to integers and store in the slice
	for key, portStr := range portMap {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			log.Printf("Error converting port to integer for %s: %v", key, err)
			continue
		}
		portInfos = append(portInfos, portInfo{key, port})
	}

	// Sort the slice by port number
	sort.Slice(portInfos, func(i, j int) bool {
		return portInfos[i].port < portInfos[j].port
	})

	// Print results
	for _, info := range portInfos {
		fmt.Printf("%s: %d\n", info.key, info.port)
	}

	return nil
}
