package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

func getImageTag(paramName string) (string, error) {
	// Load the AWS SDK configuration
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-east-2"))
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
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-2"),
	})
	if err != nil {
		log.Fatalf("Error creating session: %v", err)
	}

	// Create Secrets Manager client
	svc := secretsmanager.New(sess)

	secretMap := make(map[string]string)

	for _, secretName := range secretNames {
		// Get secret value
		input := &secretsmanager.GetSecretValueInput{
			SecretId: aws.String(secretName),
		}
		result, err := svc.GetSecretValue(input)
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
