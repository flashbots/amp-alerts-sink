package types

type AlertmanagerMessage struct {
	Alerts            []AlertmanagerAlert `json:"alerts"`
	CommonAnnotations map[string]string   `json:"commonAnnotations"`
	CommonLabels      map[string]string   `json:"commonLabels"`
	Status            string              `json:"status"`
}
