package app

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/seredavin/helmfile-validate/pkg/helmfile/testutil"
)

// TestFormatAsTable tests the FormatAsTable function.
func TestFormatAsTable(t *testing.T) {
	h := []*HelmRelease{
		{
			Name:      "test",
			Namespace: "test",
			Enabled:   true,
			Installed: true,
			Labels:    "test",
			Chart:     "test",
			Version:   "test",
		},
		{
			Name:      "test1",
			Namespace: "test2",
			Enabled:   false,
			Installed: false,
			Labels:    "test1",
			Chart:     "test1",
			Version:   "test1",
		},
	}

	tableoutput := "testdata/formatters/tableoutput"
	expectd, err := os.ReadFile(tableoutput)
	if err != nil {
		t.Errorf("error reading %s: %v", tableoutput, err)
	}

	result, err := testutil.CaptureStdout(func() {
		if err := FormatAsTable(h); err != nil {
			t.Errorf("FormatAsTable failed: %v", err)
		}
	})

	assert.NoError(t, err)

	if result != string(expectd) {
		t.Errorf("FormatAsTable() = %v, want %v", result, string(expectd))
	}
}

func TestFormatAsJson(t *testing.T) {
	h := []*HelmRelease{
		{
			Name:      "test",
			Namespace: "test",
			Enabled:   true,
			Installed: true,
			Labels:    "test",
			Chart:     "test",
			Version:   "test",
		},
		{
			Name:      "test1",
			Namespace: "test2",
			Enabled:   false,
			Installed: false,
			Labels:    "test1",
			Chart:     "test1",
			Version:   "test1",
		},
	}
	output := "testdata/formatters/jsonoutput"
	expectd, err := os.ReadFile(output)
	if err != nil {
		t.Errorf("error reading %s: %v", output, err)
	}
	result, err := testutil.CaptureStdout(func() {
		if err := FormatAsJson(h); err != nil {
			t.Errorf("FormatAsJson failed: %v", err)
		}
	})

	assert.NoError(t, err)

	if result != string(expectd) {
		t.Errorf("FormatAsJson() = %v, want %v", result, string(expectd))
	}
}
