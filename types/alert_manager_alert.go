package types

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"slices"
)

type AlertmanagerAlert struct {
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`
	StartsAt    string            `json:"startsAt"`
	Status      string            `json:"status"`
}

// IncidentDedupKey computes the hash of alert's labels only.
func (a AlertmanagerAlert) IncidentDedupKey() string {
	sum := sha256.New()

	writeMap(sum, a.Labels)
	writeString(sum, a.StartsAt)

	return hex.EncodeToString(sum.Sum(nil))
}

// MessageDedupKey computes the hash of alert's labels and annotations.
func (a AlertmanagerAlert) MessageDedupKey() string {
	sum := sha256.New()

	writeMap(sum, a.Annotations)
	writeMap(sum, a.Labels)
	writeString(sum, a.StartsAt)
	writeString(sum, a.Status)

	return hex.EncodeToString(sum.Sum(nil))
}

// writeMap writes the map to hasher in a deterministic order.
func writeMap(sum io.Writer, m map[string]string) {
	sortedKeys := make([]string, 0, len(m))
	for k := range m {
		sortedKeys = append(sortedKeys, k)
	}
	slices.Sort(sortedKeys)

	for _, k := range sortedKeys {
		writeString(sum, k)
		writeString(sum, m[k])
	}
}

// writeString writes the string to hasher with non-printable delimiter.
func writeString(sum io.Writer, s string) {
	io.WriteString(sum, s)
	sum.Write([]byte{255})
}
