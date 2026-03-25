package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/andyfcx/observer/agent/internal/discovery"
)

const sampleCrontab = `# This is a comment
CRON_TZ=Asia/Taipei

# Daily backup
0 2 * * * root /usr/local/bin/backup.sh --full > /dev/null 2>&1

# Every 5 minutes collect
*/5 * * * * www-data /usr/bin/python3 /opt/app/collect.py

# Weekly cleanup
0 0 * * 0 app /opt/app/cleanup.sh 2>&1
`

// parseCrontabFile is a test helper that parses a single file directly.
func parseCrontabFile(t *testing.T, content string) []*discovery.DiscoveredJob {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-crontab")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	jobs, err := discovery.ParseCrontabFile(path, true)
	require.NoError(t, err)
	return jobs
}

func TestParseCrontab(t *testing.T) {
	jobs := parseCrontabFile(t, sampleCrontab)

	// We expect 3 jobs from our sample file.
	require.Len(t, jobs, 3, "expected 3 parsed jobs")

	assert.Equal(t, "cron", jobs[0].SourceType)
	assert.Equal(t, "0 2 * * *", jobs[0].Schedule)
	assert.Equal(t, "root", jobs[0].User)
	assert.Equal(t, "/usr/local/bin/backup.sh --full", jobs[0].NormalizedCommand)
	assert.Equal(t, "Asia/Taipei", jobs[0].Timezone)
	assert.True(t, jobs[0].Enabled)
}

func TestNormalization(t *testing.T) {
	cases := []struct {
		content  string
		wantNorm string
	}{
		{
			// Redirection stripped
			content:  "0 * * * * root /bin/check.sh > /dev/null 2>&1\n",
			wantNorm: "/bin/check.sh",
		},
		{
			// No redirection
			content:  "5 4 * * * www /usr/bin/php /var/www/cron.php\n",
			wantNorm: "/usr/bin/php /var/www/cron.php",
		},
	}
	for _, c := range cases {
		jobs := parseCrontabFile(t, c.content)
		require.Len(t, jobs, 1)
		assert.Equal(t, c.wantNorm, jobs[0].NormalizedCommand, "content: %q", c.content)
	}
}

func TestCommandHashStability(t *testing.T) {
	// Same command should always produce same hash.
	content := "0 1 * * * root /usr/local/bin/backup.sh\n"
	jobs1 := parseCrontabFile(t, content)
	jobs2 := parseCrontabFile(t, content)

	require.Len(t, jobs1, 1)
	require.Len(t, jobs2, 1)
	assert.Equal(t, jobs1[0].CommandHash, jobs2[0].CommandHash)
}

func TestScannerUsesExtraDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-jobs")
	require.NoError(t, os.WriteFile(path, []byte(sampleCrontab), 0644))

	scanner := &discovery.CronScanner{
		ExtraCronDirs:   []string{dir},
		SkipSystemCrons: true, // use test-only flag to isolate from system crontabs
	}
	jobs, err := scanner.Scan()
	require.NoError(t, err)
	require.Len(t, jobs, 3, "expected 3 jobs from ExtraCronDirs")
}
