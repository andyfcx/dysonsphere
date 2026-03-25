package service

import (
	"context"
	"log/slog"

	"github.com/andyfcx/observer/server/internal/domain"
	"github.com/andyfcx/observer/server/internal/repository"
)

// MetricService processes data metric probe results.
type MetricService struct {
	metrics *repository.MetricRepo
}

func NewMetricService(metrics *repository.MetricRepo) *MetricService {
	return &MetricService{metrics: metrics}
}

// BatchReport stores a batch of data metrics from an agent probe run.
func (s *MetricService) BatchReport(ctx context.Context, hostID string, metrics []*domain.DataMetric) ([]*domain.DataMetric, error) {
	result := make([]*domain.DataMetric, 0, len(metrics))
	for _, m := range metrics {
		m.HostID = hostID
		saved, err := s.metrics.Insert(ctx, m)
		if err != nil {
			slog.Warn("failed to insert metric", "host_id", hostID, "name", m.MetricName, "err", err)
			continue
		}
		result = append(result, saved)
	}
	slog.Info("batch metrics processed", "host_id", hostID, "count", len(result))
	return result, nil
}

// List returns paginated metrics.
func (s *MetricService) List(ctx context.Context, limit, offset int) ([]*domain.DataMetric, error) {
	return s.metrics.List(ctx, limit, offset)
}
