package config

type PagerDuty struct {
	IntegrationKey string `yaml:"integration_key"`
}

func (s PagerDuty) Enabled() bool {
	return s.IntegrationKey != ""
}
