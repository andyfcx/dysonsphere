package alerting_test

import (
	"testing"

	"github.com/andyfcx/observer/server/internal/alerting"
	"github.com/stretchr/testify/assert"
)

// TestRuleNames verifies the exported rule name constants are non-empty.
func TestRuleNames(t *testing.T) {
	assert.NotEmpty(t, alerting.RuleStaleHeartbeat)
	assert.NotEmpty(t, alerting.RuleRepeatedFailures)
}

// Integration tests for the evaluator require a live DB.
// TODO: add testcontainers-based integration tests.
