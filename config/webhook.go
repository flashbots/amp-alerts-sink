package config

type Webhook struct {
	URL      string `yaml:"url"`
	Method   string `yaml:"method"`
	SendBody bool   `yaml:"send_body"`
}

func (w *Webhook) Enabled() bool {
	return w.URL != ""
}
