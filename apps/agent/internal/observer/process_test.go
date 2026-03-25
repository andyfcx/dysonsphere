package observer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/andyfcx/observer/agent/internal/observer"
)

func TestMatchJobs(t *testing.T) {
	snapshots := []*observer.ProcessSnapshot{
		{PID: 100, Cmdline: "/usr/bin/python3 /opt/app/collect.py --run"},
		{PID: 200, Cmdline: "/bin/sh -c /usr/local/bin/backup.sh --full"},
		{PID: 300, Cmdline: "/usr/sbin/sshd -D"},
	}

	jobCommands := map[string]string{
		"hash_collect": "/usr/bin/python3 /opt/app/collect.py",
		"hash_backup":  "/usr/local/bin/backup.sh",
	}

	matched := observer.MatchJobs(snapshots, jobCommands)

	// Should match python3 and backup, not sshd.
	assert.Len(t, matched, 2)

	hashSet := make(map[string]bool)
	for _, m := range matched {
		hashSet[m.JobHash] = true
		assert.Greater(t, m.Confidence, 0.0)
		assert.NotEmpty(t, m.Evidence)
	}
	assert.True(t, hashSet["hash_collect"])
	assert.True(t, hashSet["hash_backup"])
}

func TestMatchJobsNoMatch(t *testing.T) {
	snapshots := []*observer.ProcessSnapshot{
		{PID: 1, Cmdline: "/usr/sbin/systemd"},
	}
	jobs := map[string]string{"hash1": "/opt/myapp/worker"}
	matched := observer.MatchJobs(snapshots, jobs)
	assert.Empty(t, matched)
}
