package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andyfcx/observer/server/internal/domain"
)

// StatsRepo runs aggregate queries over execution history.
type StatsRepo struct {
	db *pgxpool.Pool
}

func NewStatsRepo(db *pgxpool.Pool) *StatsRepo {
	return &StatsRepo{db: db}
}

// GetWindowStats returns aggregate execution counts since `since`.
func (r *StatsRepo) GetWindowStats(ctx context.Context, since time.Time, label string) (*domain.WindowStats, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*)                                         AS total,
			COUNT(*) FILTER (WHERE status = 'success')      AS success,
			COUNT(*) FILTER (WHERE status = 'failed')       AS failed,
			COUNT(*) FILTER (WHERE status = 'partial')      AS partial,
			COUNT(*) FILTER (WHERE status = 'unknown')      AS unknown,
			COUNT(*) FILTER (WHERE status = 'missed')       AS missed
		FROM executions
		WHERE created_at >= $1 AND status != 'running'
	`, since)

	var s domain.WindowStats
	s.Window = label
	s.Since = since.UTC().Format(time.RFC3339)
	if err := row.Scan(&s.Total, &s.Success, &s.Failed, &s.Partial, &s.Unknown, &s.Missed); err != nil {
		return nil, fmt.Errorf("window stats: %w", err)
	}
	if s.Total > 0 {
		s.SuccessRate = float64(s.Success) / float64(s.Total) * 100
	}
	return &s, nil
}

// GetFailureTrend returns time-bucketed execution counts since `since`.
// bucketSeconds is the bucket width in seconds (e.g. 3600=1h, 21600=6h, 86400=1d).
func (r *StatsRepo) GetFailureTrend(ctx context.Context, since time.Time, bucketSeconds int) ([]domain.TrendPoint, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			to_timestamp(floor(extract(epoch from created_at) / $2) * $2) AS bucket,
			COUNT(*)                                         AS total,
			COUNT(*) FILTER (WHERE status = 'success')      AS success,
			COUNT(*) FILTER (WHERE status = 'failed')       AS failed,
			COUNT(*) FILTER (WHERE status IN ('unknown','partial')) AS unknown
		FROM executions
		WHERE created_at >= $1 AND status != 'running'
		GROUP BY 1
		ORDER BY 1
	`, since, float64(bucketSeconds))
	if err != nil {
		return nil, fmt.Errorf("failure trend: %w", err)
	}
	defer rows.Close()

	points := make([]domain.TrendPoint, 0)
	for rows.Next() {
		var p domain.TrendPoint
		if err := rows.Scan(&p.Bucket, &p.Total, &p.Success, &p.Failed, &p.Unknown); err != nil {
			return nil, fmt.Errorf("scan trend point: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetJobSuccessRates returns per-job execution success rates since `since`,
// ordered by failure count descending.
func (r *StatsRepo) GetJobSuccessRates(ctx context.Context, since time.Time) ([]domain.JobSuccessRate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			e.job_id::text,
			j.host_id::text,
			COUNT(*)                                        AS total,
			COUNT(*) FILTER (WHERE e.status = 'success')   AS success,
			COUNT(*) FILTER (WHERE e.status = 'failed')    AS failed,
			COALESCE(cjs.consecutive_fail, 0)              AS consecutive_fail,
			MAX(e.created_at)                              AS last_run_at
		FROM executions e
		JOIN jobs j ON j.id = e.job_id
		LEFT JOIN current_job_states cjs ON cjs.job_id = e.job_id
		WHERE e.created_at >= $1 AND e.job_id IS NOT NULL AND e.status != 'running'
		GROUP BY e.job_id, j.host_id, cjs.consecutive_fail
		ORDER BY COUNT(*) FILTER (WHERE e.status = 'failed') DESC, total DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("job success rates: %w", err)
	}
	defer rows.Close()

	rates := make([]domain.JobSuccessRate, 0)
	for rows.Next() {
		var jsr domain.JobSuccessRate
		if err := rows.Scan(
			&jsr.JobID, &jsr.HostID,
			&jsr.Total, &jsr.Success, &jsr.Failed,
			&jsr.ConsecutiveFail, &jsr.LastRunAt,
		); err != nil {
			return nil, fmt.Errorf("scan job success rate: %w", err)
		}
		if jsr.Total > 0 {
			jsr.SuccessRate = float64(jsr.Success) / float64(jsr.Total) * 100
		}
		rates = append(rates, jsr)
	}
	return rates, rows.Err()
}

// GetHostFailureCounts returns per-host failure counts since `since`,
// ordered by failure count descending.
func (r *StatsRepo) GetHostFailureCounts(ctx context.Context, since time.Time) ([]domain.HostFailureCount, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			host_id::text,
			COUNT(*)                                    AS total,
			COUNT(*) FILTER (WHERE status = 'failed')  AS failed
		FROM executions
		WHERE created_at >= $1 AND status != 'running'
		GROUP BY host_id
		ORDER BY failed DESC, total DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("host failure counts: %w", err)
	}
	defer rows.Close()

	counts := make([]domain.HostFailureCount, 0)
	for rows.Next() {
		var c domain.HostFailureCount
		if err := rows.Scan(&c.HostID, &c.Total, &c.Failed); err != nil {
			return nil, fmt.Errorf("scan host failure count: %w", err)
		}
		if c.Total > 0 {
			c.FailureRate = float64(c.Failed) / float64(c.Total) * 100
		}
		counts = append(counts, c)
	}
	return counts, rows.Err()
}

// GetJobRecoveryStats returns failure episode analysis for all jobs that have
// ever had at least one failure. Uses the gaps-and-islands pattern to detect
// consecutive failure runs and measure time-to-recovery.
func (r *StatsRepo) GetJobRecoveryStats(ctx context.Context) ([]domain.JobRecoverySummary, error) {
	rows, err := r.db.Query(ctx, `
		WITH ordered_execs AS (
			SELECT
				job_id,
				status,
				created_at,
				ROW_NUMBER() OVER (PARTITION BY job_id ORDER BY created_at)          AS rn,
				ROW_NUMBER() OVER (
					PARTITION BY job_id,
						CASE WHEN status IN ('failed','unknown','missed') THEN 1 ELSE 0 END
					ORDER BY created_at
				)                                                                    AS type_rn,
				CASE WHEN status IN ('failed','unknown','missed') THEN 1 ELSE 0 END  AS is_bad
			FROM executions
			WHERE job_id IS NOT NULL
		),
		islands AS (
			SELECT job_id, created_at, is_bad, rn - type_rn AS island_id
			FROM ordered_execs
		),
		failure_islands AS (
			SELECT
				job_id,
				MIN(created_at) AS episode_start,
				MAX(created_at) AS episode_end,
				COUNT(*)        AS failure_count
			FROM islands
			WHERE is_bad = 1
			GROUP BY job_id, island_id
		),
		recovery AS (
			SELECT DISTINCT ON (fi.job_id, fi.episode_start)
				fi.job_id,
				fi.episode_start,
				fi.episode_end,
				fi.failure_count,
				e.created_at                                                        AS recovered_at,
				EXTRACT(EPOCH FROM (e.created_at - fi.episode_start))               AS recovery_seconds
			FROM failure_islands fi
			LEFT JOIN executions e
				   ON e.job_id = fi.job_id
				  AND e.created_at > fi.episode_end
				  AND e.status = 'success'
			ORDER BY fi.job_id, fi.episode_start, e.created_at
		)
		SELECT
			r.job_id::text,
			j.host_id::text,
			r.episode_start,
			r.episode_end,
			r.failure_count,
			r.recovered_at,
			r.recovery_seconds
		FROM recovery r
		JOIN jobs j ON j.id = r.job_id
		ORDER BY r.job_id, r.episode_start DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("job recovery stats: %w", err)
	}
	defer rows.Close()

	summaryMap := map[string]*domain.JobRecoverySummary{}
	var jobOrder []string

	for rows.Next() {
		var jobID, hostID string
		var episodeStart, episodeEnd time.Time
		var failureCount int64
		var recoveredAt *time.Time
		var recoverySeconds *float64

		if err := rows.Scan(
			&jobID, &hostID,
			&episodeStart, &episodeEnd, &failureCount,
			&recoveredAt, &recoverySeconds,
		); err != nil {
			return nil, fmt.Errorf("scan recovery row: %w", err)
		}

		if _, ok := summaryMap[jobID]; !ok {
			summaryMap[jobID] = &domain.JobRecoverySummary{
				JobID:    jobID,
				HostID:   hostID,
				Episodes: []domain.RecoveryEpisode{},
			}
			jobOrder = append(jobOrder, jobID)
		}
		summaryMap[jobID].Episodes = append(summaryMap[jobID].Episodes, domain.RecoveryEpisode{
			EpisodeStart:    episodeStart,
			EpisodeEnd:      episodeEnd,
			FailureCount:    failureCount,
			RecoveredAt:     recoveredAt,
			RecoverySeconds: recoverySeconds,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]domain.JobRecoverySummary, 0, len(summaryMap))
	for _, jobID := range jobOrder {
		s := summaryMap[jobID]

		// Episodes are ordered DESC by start time; [0] is the most recent.
		if len(s.Episodes) > 0 {
			latest := s.Episodes[0]
			if latest.RecoveredAt == nil {
				s.IsCurrentlyFailing = true
				s.CurrentEpisodeSince = &latest.EpisodeStart
				s.CurrentEpisodeFails = latest.FailureCount
			} else {
				s.LastRecoveredAt = latest.RecoveredAt
			}
		}

		// Average recovery seconds across all resolved episodes.
		var totalSec float64
		var count int
		for _, ep := range s.Episodes {
			if ep.RecoverySeconds != nil {
				totalSec += *ep.RecoverySeconds
				count++
			}
		}
		if count > 0 {
			avg := totalSec / float64(count)
			s.AvgRecoverySeconds = &avg
		}

		result = append(result, *s)
	}
	return result, nil
}
