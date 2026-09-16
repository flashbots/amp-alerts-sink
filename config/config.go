package config

type Config struct {
	DynamoDB  *DynamoDB  `yaml:"dynamo_db"`
	Log       *Log       `yaml:"log"`
	Processor *Processor `yaml:"processor"`

	A2A       *A2A       `yaml:"a2a"`
	PagerDuty *PagerDuty `yaml:"pagerduty"`
	Slack     *Slack     `yaml:"slack"`
	Webhook   *Webhook   `yaml:"webhook"`
}

func New() *Config {
	return &Config{
		DynamoDB:  &DynamoDB{},
		Log:       &Log{},
		Processor: &Processor{},

		A2A:       &A2A{},
		PagerDuty: &PagerDuty{},
		Slack:     &Slack{Channel: &SlackChannel{}},
		Webhook:   &Webhook{},
	}
}
