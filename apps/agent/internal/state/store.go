// Package state provides local persistent storage for the agent.
// Uses bbolt (an embedded key-value store) to buffer events when the
// central server is temporarily unavailable.
package state

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketExecutions = []byte("executions")
	bucketMetrics    = []byte("metrics")
	bucketMeta       = []byte("meta")
	bucketJobs       = []byte("jobs")
	bucketLocalRuns  = []byte("local_runs")
)

// Store wraps a bbolt database for local agent state.
type Store struct {
	db *bolt.DB
}

// Open opens (or creates) the bbolt state file at the given path.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}

	// Ensure buckets exist.
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketExecutions, bucketMetrics, bucketMeta, bucketJobs, bucketLocalRuns} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("init buckets: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// PendingExecution is a buffered execution event awaiting upload.
type PendingExecution struct {
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

// PendingMetric is a buffered metric awaiting upload.
type PendingMetric struct {
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

// DiscoveredJob stores the latest locally known job definition.
type DiscoveredJob struct {
	CommandHash       string    `json:"command_hash"`
	Schedule          string    `json:"schedule"`
	Timezone          string    `json:"timezone"`
	User              string    `json:"user"`
	RawCommand        string    `json:"raw_command"`
	NormalizedCommand string    `json:"normalized_command"`
	SourceFile        string    `json:"source_file"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// LocalExecution records the locally known lifecycle of a cron execution.
type LocalExecution struct {
	ID                 string          `json:"id"`
	JobID              string          `json:"job_id,omitempty"`
	CommandHash        string          `json:"command_hash,omitempty"`
	Command            string          `json:"command"`
	Schedule           string          `json:"schedule,omitempty"`
	Status             string          `json:"status"`
	Trigger            string          `json:"trigger"`
	ConfidenceScore    float64         `json:"confidence_score"`
	StartedAt          time.Time       `json:"started_at"`
	FinishedAt         *time.Time      `json:"finished_at,omitempty"`
	DurationSeconds    *float64        `json:"duration_seconds,omitempty"`
	PID                *int32          `json:"pid,omitempty"`
	ExitCode           *int            `json:"exit_code,omitempty"`
	Evidence           json.RawMessage `json:"evidence,omitempty"`
	LastUpdatedAt      time.Time       `json:"last_updated_at"`
}

// SaveExecution buffers an execution event.
func (s *Store) SaveExecution(id string, payload any) error {
	return s.save(bucketExecutions, id, &PendingExecution{
		ID:        id,
		CreatedAt: time.Now(),
		Payload:   mustMarshal(payload),
	})
}

// SaveMetric buffers a metric event.
func (s *Store) SaveMetric(id string, payload any) error {
	return s.save(bucketMetrics, id, &PendingMetric{
		ID:        id,
		CreatedAt: time.Now(),
		Payload:   mustMarshal(payload),
	})
}

// PendingExecutions returns all buffered execution events.
func (s *Store) PendingExecutions() ([]*PendingExecution, error) {
	var items []*PendingExecution
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketExecutions).ForEach(func(k, v []byte) error {
			var item PendingExecution
			if err := json.Unmarshal(v, &item); err != nil {
				return err
			}
			items = append(items, &item)
			return nil
		})
	})
	return items, err
}

// PendingMetrics returns all buffered metric events.
func (s *Store) PendingMetrics() ([]*PendingMetric, error) {
	var items []*PendingMetric
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetrics).ForEach(func(k, v []byte) error {
			var item PendingMetric
			if err := json.Unmarshal(v, &item); err != nil {
				return err
			}
			items = append(items, &item)
			return nil
		})
	})
	return items, err
}

// DeleteExecution removes a buffered execution after successful upload.
func (s *Store) DeleteExecution(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketExecutions).Delete([]byte(id))
	})
}

// DeleteMetric removes a buffered metric after successful upload.
func (s *Store) DeleteMetric(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetrics).Delete([]byte(id))
	})
}

// SetMeta stores a simple string value in the meta bucket.
func (s *Store) SetMeta(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put([]byte(key), []byte(value))
	})
}

// GetMeta retrieves a string value from the meta bucket.
func (s *Store) GetMeta(key string) (string, error) {
	var value string
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketMeta).Get([]byte(key))
		if v != nil {
			value = string(v)
		}
		return nil
	})
	return value, err
}

// SaveJob stores the latest known job definition keyed by command hash.
func (s *Store) SaveJob(job *DiscoveredJob) error {
	job.UpdatedAt = time.Now()
	return s.save(bucketJobs, job.CommandHash, job)
}

// ListJobs returns all locally known discovered jobs.
func (s *Store) ListJobs() ([]*DiscoveredJob, error) {
	var items []*DiscoveredJob
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketJobs).ForEach(func(k, v []byte) error {
			var item DiscoveredJob
			if err := json.Unmarshal(v, &item); err != nil {
				return err
			}
			items = append(items, &item)
			return nil
		})
	})
	return items, err
}

// SaveLocalExecution creates or updates a local execution record.
func (s *Store) SaveLocalExecution(exec *LocalExecution) error {
	exec.LastUpdatedAt = time.Now()
	return s.save(bucketLocalRuns, exec.ID, exec)
}

// ListLocalExecutionsSince returns local execution records updated since the given time.
func (s *Store) ListLocalExecutionsSince(since time.Time) ([]*LocalExecution, error) {
	var items []*LocalExecution
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketLocalRuns).ForEach(func(k, v []byte) error {
			var item LocalExecution
			if err := json.Unmarshal(v, &item); err != nil {
				return err
			}
			if item.LastUpdatedAt.After(since) || item.StartedAt.After(since) {
				items = append(items, &item)
			}
			return nil
		})
	})
	return items, err
}

func (s *Store) save(bucket []byte, id string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucket).Put([]byte(id), b)
	})
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
