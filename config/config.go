package config

type Config struct {
	DynamoDB  *DynamoDB  `yaml:"dynamo_db"`
	Log       *Log       `yaml:"log"`
	Processor *Processor `yaml:"processor"`
	Slack     *Slack     `yaml:"slack"`
}

func New() *Config {
	return &Config{
		DynamoDB:  &DynamoDB{},
		Log:       &Log{},
		Processor: &Processor{},

		Slack: &Slack{
			Channel: &SlackChannel{},
		},
	}
}
