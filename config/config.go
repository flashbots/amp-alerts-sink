package config

type Config struct {
	DynamoDB  *DynamoDB  `yaml:"dynamo_db"`
	Log       *Log       `yaml:"log"`
	Processor *Processor `yaml:"processor"`

	PagerDuty *PagerDuty `yaml:"pagerduty"`
	Slack     *Slack     `yaml:"slack"`
}

func New() *Config {
	return &Config{
		DynamoDB:  &DynamoDB{},
		Log:       &Log{},
		Processor: &Processor{},

		PagerDuty: &PagerDuty{},
		Slack:     &Slack{Channel: &SlackChannel{}},
	}
}
