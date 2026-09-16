package config

import "time"

const DefaultA2APrompt = "Investigate this alert (JSON): {{ toJSON .Alert }}"

type A2A struct {
	BearerToken    string        `yaml:"bearer_token"`
	PromptTemplate string        `yaml:"prompt_template"`
	PromptUrl      string        `yaml:"prompt_url"`
	Timeout        time.Duration `yaml:"timeout"`
}

func (a *A2A) Enabled() bool {
	return a.PromptUrl != ""
}
