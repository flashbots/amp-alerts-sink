package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/flashbots/amp-alerts-sink/config"
	"github.com/flashbots/amp-alerts-sink/processor"
	"github.com/flashbots/amp-alerts-sink/secret"
	"github.com/urfave/cli/v2"

	awslambda "github.com/aws/aws-lambda-go/lambda"
)

const (
	categoryDynamoDB  = "DYNAMO DB:"
	categoryProcessor = "PROCESSOR:"
	categorySlack     = "PUBLISHER SLACK:"
	categoryPagerDuty = "PUBLISHER PAGERDUTY:"
)

var (
	errProcessorInvalidLabelMatch  = errors.New("invalid label match (must be 'label=value')")
	errSlackChannelIDNotConfigured = errors.New("slack channel ID must be configured")
)

func CommandLambda(cfg *config.Config) *cli.Command {
	envPrefixDynamoDB := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(categoryDynamoDB, " ", "_"), ":", "")) + "_"
	envPrefixProcessor := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(categoryProcessor, " ", "_"), ":", "")) + "_"
	envPrefixSlack := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(categorySlack, " ", "_"), ":", "")) + "_"
	envPrefixPagerDuty := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(categoryPagerDuty, " ", "_"), ":", "")) + "_"

	cliPrefixDynamoDB := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(categoryDynamoDB, " ", "-"), ":", "")) + "-"
	cliPrefixProcessor := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(categoryProcessor, " ", "-"), ":", "")) + "-"
	cliPrefixSlack := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(categorySlack, " ", "-"), ":", "")) + "-"
	cliPrefixPagerDuty := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(categoryPagerDuty, " ", "-"), ":", "")) + "-"

	envSlackToken := envPrefix + envPrefixSlack + "TOKEN"
	envPagerDutyIntegrationKey := envPrefix + envPrefixPagerDuty + "INTEGRATION_KEY"

	rawProcessorIgnoreRules := &cli.StringSlice{}
	rawProcessorMatchLabels := &cli.StringSlice{}

	flagsDB := []cli.Flag{
		&cli.StringFlag{
			Category:    categoryDynamoDB,
			Destination: &cfg.DynamoDB.Name,
			EnvVars:     []string{envPrefix + envPrefixDynamoDB + "NAME"},
			Name:        cliPrefixDynamoDB + "name",
			Required:    true,
			Usage:       "`name` of Dynamo DB to keep track of alert statuses with",
		},
	}

	flagsProcessor := []cli.Flag{
		&cli.StringSliceFlag{
			Category:    categoryProcessor,
			EnvVars:     []string{envPrefix + envPrefixProcessor + "IGNORE_RULES"},
			Destination: rawProcessorIgnoreRules,
			Name:        cliPrefixProcessor + "ignore-rules",
			Usage:       "comma-separated list of `rule`s to ignore",
		},

		&cli.StringSliceFlag{
			Category:    categoryProcessor,
			EnvVars:     []string{envPrefix + envPrefixProcessor + "MATCH_LABELS"},
			Destination: rawProcessorMatchLabels,
			Name:        cliPrefixProcessor + "match-labels",
			Usage:       "comma-separated list of `label=value` pairs to match",
		},
	}

	flagsSlack := []cli.Flag{
		&cli.StringFlag{
			Category:    categorySlack,
			Destination: &cfg.Slack.Channel.ID,
			EnvVars:     []string{envPrefix + envPrefixSlack + "CHANNEL_ID"},
			Name:        cliPrefixSlack + "channel-id",
			Usage:       "slack channel `ID` to publish alerts to",
		},

		&cli.StringFlag{
			Category:    categorySlack,
			Destination: &cfg.Slack.Token,
			EnvVars:     []string{envSlackToken},
			Name:        cliPrefixSlack + "token",
			Usage:       "slack API `token` (either raw token, or ARN of secret manager)",
		},
	}

	flagsPagerDuty := []cli.Flag{
		&cli.StringFlag{
			Category:    categoryPagerDuty,
			Destination: &cfg.PagerDuty.IntegrationKey,
			EnvVars:     []string{envPagerDutyIntegrationKey},
			Name:        cliPrefixPagerDuty + "integration-key",
			Usage:       "pagerduty `integration key` to publish alerts to",
		},
	}

	flags := slices.Concat(
		flagsDB,
		flagsProcessor,
		flagsSlack,
		flagsPagerDuty,
	)

	return &cli.Command{
		Name:  "lambda",
		Usage: "Run lambda handler (default)",
		Flags: flags,

		Before: func(_ *cli.Context) error {
			var err error

			if cfg.Slack.Token != "" {
				if cfg.Slack.Channel.ID == "" {
					return errSlackChannelIDNotConfigured
				}

				cfg.Slack.Token, err = stringOrLoadFromSecretsmanager(cfg.Slack.Token, envSlackToken)
				if err != nil {
					return err
				}
			}

			cfg.PagerDuty.IntegrationKey, err = stringOrLoadFromSecretsmanager(
				cfg.PagerDuty.IntegrationKey, envPagerDutyIntegrationKey)
			if err != nil {
				return err
			}

			{ // parse the list of ignored rules
				processorIgnoreRules := rawProcessorIgnoreRules.Value()
				if len(processorIgnoreRules) > 0 {
					cfg.Processor.IgnoreRules = processorIgnoreRules
				}
			}

			{ // parse the list of matched labels
				processorMatchLabelsList := rawProcessorMatchLabels.Value()
				if len(processorMatchLabelsList) > 0 {
					processorMatchLabels := make(map[string]string, len(processorMatchLabelsList))
					for _, pair := range processorMatchLabelsList {
						parts := strings.Split(pair, "=")
						if len(parts) != 2 {
							return fmt.Errorf("%w: %s",
								errProcessorInvalidLabelMatch, pair,
							)
						}
						k := strings.TrimSpace(parts[0])
						v := strings.TrimSpace(parts[1])
						processorMatchLabels[k] = v
					}
					cfg.Processor.MatchLabels = processorMatchLabels
				}
			}

			return nil
		},

		Action: func(clictx *cli.Context) error {
			p, err := processor.New(cfg)
			if err != nil {
				return err
			}
			awslambda.Start(p.ProcessSnsEvent)
			return nil
		},
	}
}

// stringOrLoadFromSecretsmanager either returns s as-is, or looks up
// the Secrets Manager secret by ARN and looks up the key in object
func stringOrLoadFromSecretsmanager(s, key string) (string, error) {
	if !strings.HasPrefix(s, "arn:aws:secretsmanager:") {
		return s, nil
	}

	return secret.AWSValue(s, key)
}
