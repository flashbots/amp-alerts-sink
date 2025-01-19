package config

type Processor struct {
	IgnoreRules []string          `yaml:"ignore_rules"`
	MatchLabels map[string]string `yaml:"match_labels"`
}
