package secret

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

const (
	timeout      = time.Second
	versionStage = "AWSCURRENT"
)

var (
	ErrSecretEmpty             = errors.New("no secret or secret is empty")
	ErrSecretFailedToUnmarshal = errors.New("failed to unmarshal the secret")
	ErrSecretInvalidArn        = errors.New("secret's ARN seems to be corrupt")
)

func AWS(arn string) (
	map[string]string, error,
) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	if len(cfg.Region) == 0 {
		// 0   1   2              3         4          5      6
		// arn:aws:secretsmanager:${REGION}:${ACCOUNT}:secret:${SECRET}
		parts := strings.Split(arn, ":")
		if len(parts) != 7 {
			return nil, fmt.Errorf("%w: %s",
				ErrSecretInvalidArn, arn,
			)
		}
		cfg.Region = parts[3]
	}

	cli := secretsmanager.NewFromConfig(cfg)

	res, err := cli.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(arn),
		VersionStage: aws.String(versionStage),
	})
	if err != nil {
		return nil, err
	}
	if res.SecretString == nil || len(*res.SecretString) == 0 {
		return nil, ErrSecretEmpty
	}

	var secrets map[string]string
	err = json.Unmarshal([]byte(*res.SecretString), &secrets)
	if err != nil {
		return nil, fmt.Errorf("%w: %w: %s",
			ErrSecretFailedToUnmarshal, err, *res.SecretString,
		)
	}

	return secrets, nil
}
