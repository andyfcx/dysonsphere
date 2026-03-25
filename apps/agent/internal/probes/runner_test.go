package probes_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/andyfcx/observer/agent/internal/config"
	"github.com/andyfcx/observer/agent/internal/probes"
)

func TestFileFreshnessProbe_Fresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0644))

	cfg := config.ProbeConfig{
		Name: "test_file",
		Type: "file_freshness",
		File: &config.FileProbe{
			Path:          path,
			MaxAgeSeconds: 60,
		},
	}
	runner := probes.NewRunner([]config.ProbeConfig{cfg})
	results := runner.RunAll(context.Background())
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "test_file", r.ProbeName)
	assert.Equal(t, "ok", r.Status)
	assert.GreaterOrEqual(t, r.Value, 0.0)
	assert.Nil(t, r.Error)
}

func TestFileFreshnessProbe_Stale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.txt")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0644))

	// Backdate the file modification time.
	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(path, oldTime, oldTime))

	cfg := config.ProbeConfig{
		Name: "stale_file",
		Type: "file_freshness",
		File: &config.FileProbe{
			Path:          path,
			MaxAgeSeconds: 3600, // 1 hour max
		},
	}
	runner := probes.NewRunner([]config.ProbeConfig{cfg})
	results := runner.RunAll(context.Background())
	require.Len(t, results, 1)
	assert.Equal(t, "warning", results[0].Status)
}

func TestFileFreshnessProbe_Missing(t *testing.T) {
	cfg := config.ProbeConfig{
		Name: "missing_file",
		Type: "file_freshness",
		File: &config.FileProbe{
			Path:          "/nonexistent/path/file.txt",
			MaxAgeSeconds: 60,
		},
	}
	runner := probes.NewRunner([]config.ProbeConfig{cfg})
	results := runner.RunAll(context.Background())
	require.Len(t, results, 1)
	assert.Equal(t, "critical", results[0].Status)
}

func TestCommandProbe(t *testing.T) {
	cfg := config.ProbeConfig{
		Name: "echo_probe",
		Type: "command",
		Command: &config.CommandProbe{
			Command: "echo",
			Args:    []string{"42"},
		},
	}
	runner := probes.NewRunner([]config.ProbeConfig{cfg})
	results := runner.RunAll(context.Background())
	require.Len(t, results, 1)
	assert.Equal(t, "ok", results[0].Status)
	assert.Equal(t, 42.0, results[0].Value)
}

func TestUnknownProbeType(t *testing.T) {
	cfg := config.ProbeConfig{
		Name: "bad",
		Type: "nonexistent_type",
	}
	runner := probes.NewRunner([]config.ProbeConfig{cfg})
	results := runner.RunAll(context.Background())
	require.Len(t, results, 1)
	assert.Equal(t, "unknown", results[0].Status)
	assert.NotNil(t, results[0].Error)
}
