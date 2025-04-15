package config

type Slack struct {
	Channel *SlackChannel `yaml:"channel"`
	Token   string        `yaml:"slack"`
}

func (s *Slack) Enabled() bool {
	return s.Token != "" &&
		s.Channel != nil && s.Channel.ID != ""
}
