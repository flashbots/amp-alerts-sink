package processor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/flashbots/amp-alerts-sink/types"
	"github.com/stretchr/testify/assert"
)

func TestSanitiseJsEscapedApostrophes(t *testing.T) {
	raw := strings.TrimSpace(`{
		"no-apostrophe":           "text text text",
		"single apostrophe":       "'text text text'",
		"js-escaped apostrophe":   "\'text text text\'",
		"json-escaped apostrophe": "\\'text text text\\'"
	}`)
	message := &types.AlertmanagerMessage{}
	err := json.Unmarshal([]byte(raw), message)
	assert.Equal(t, "invalid character '\\'' in string escape code", err.Error())
	raw = string(sanitiseJsEscapedApostrophes([]byte(raw)))
	err = json.Unmarshal([]byte(raw), message)
	assert.NoError(t, err)
}
