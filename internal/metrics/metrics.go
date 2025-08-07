package metrics

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics represents application metrics
type Metrics struct {
	// Queue metrics
	queueDepth          *prometheus.GaugeVec
	queueMessagesTotal  *prometheus.CounterVec
	queueMessagesFailed *prometheus.CounterVec

	// Worker metrics
	activeWorkers           *prometheus.GaugeVec
	workerMessagesProcessed *prometheus.CounterVec
	workerErrors            *prometheus.CounterVec

	// Message processing metrics
	messageProcessingDuration *prometheus.HistogramVec
	messageRetryCount         *prometheus.CounterVec

	// Consumer metrics
	consumerStatus *prometheus.GaugeVec

	mu sync.RWMutex
}

// NewMetrics creates new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		queueDepth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "queue_depth",
				Help: "Number of messages in queue",
			},
			[]string{"tenant_id", "queue_name"},
		),
		queueMessagesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "queue_messages_total",
				Help: "Total number of messages processed",
			},
			[]string{"tenant_id", "queue_name", "status"},
		),
		queueMessagesFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "queue_messages_failed",
				Help: "Total number of failed messages",
			},
			[]string{"tenant_id", "queue_name"},
		),
		activeWorkers: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "active_workers",
				Help: "Number of active workers",
			},
			[]string{"tenant_id", "queue_name"},
		),
		workerMessagesProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "worker_messages_processed",
				Help: "Number of messages processed by workers",
			},
			[]string{"tenant_id", "queue_name"},
		),
		workerErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "worker_errors",
				Help: "Number of worker errors",
			},
			[]string{"tenant_id", "queue_name"},
		),
		messageProcessingDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "message_processing_duration_seconds",
				Help:    "Time taken to process messages",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"tenant_id", "queue_name"},
		),
		messageRetryCount: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "message_retry_count",
				Help: "Number of message retries",
			},
			[]string{"tenant_id", "queue_name"},
		),
		consumerStatus: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "consumer_status",
				Help: "Consumer status (1=active, 0=inactive)",
			},
			[]string{"tenant_id", "queue_name"},
		),
	}
}

// SetQueueDepth sets the queue depth metric
func (m *Metrics) SetQueueDepth(tenantID int, queueName string, depth int) {
	m.queueDepth.WithLabelValues(
		fmt.Sprintf("%d", tenantID),
		queueName,
	).Set(float64(depth))
}

// IncrementMessageTotal increments the total messages counter
func (m *Metrics) IncrementMessageTotal(tenantID int, queueName string, status string) {
	m.queueMessagesTotal.WithLabelValues(
		fmt.Sprintf("%d", tenantID),
		queueName,
		status,
	).Inc()
}

// IncrementMessageFailed increments the failed messages counter
func (m *Metrics) IncrementMessageFailed(tenantID int, queueName string) {
	m.queueMessagesFailed.WithLabelValues(
		fmt.Sprintf("%d", tenantID),
		queueName,
	).Inc()
}

// SetActiveWorkers sets the active workers metric
func (m *Metrics) SetActiveWorkers(tenantID int, queueName string, count int) {
	m.activeWorkers.WithLabelValues(
		fmt.Sprintf("%d", tenantID),
		queueName,
	).Set(float64(count))
}

// IncrementWorkerMessagesProcessed increments the processed messages counter
func (m *Metrics) IncrementWorkerMessagesProcessed(tenantID int, queueName string) {
	m.workerMessagesProcessed.WithLabelValues(
		fmt.Sprintf("%d", tenantID),
		queueName,
	).Inc()
}

// IncrementWorkerErrors increments the worker errors counter
func (m *Metrics) IncrementWorkerErrors(tenantID int, queueName string) {
	m.workerErrors.WithLabelValues(
		fmt.Sprintf("%d", tenantID),
		queueName,
	).Inc()
}

// ObserveMessageProcessingDuration observes message processing duration
func (m *Metrics) ObserveMessageProcessingDuration(tenantID int, queueName string, duration float64) {
	m.messageProcessingDuration.WithLabelValues(
		fmt.Sprintf("%d", tenantID),
		queueName,
	).Observe(duration)
}

// IncrementMessageRetryCount increments the retry count
func (m *Metrics) IncrementMessageRetryCount(tenantID int, queueName string) {
	m.messageRetryCount.WithLabelValues(
		fmt.Sprintf("%d", tenantID),
		queueName,
	).Inc()
}

// SetConsumerStatus sets the consumer status metric
func (m *Metrics) SetConsumerStatus(tenantID int, queueName string, active bool) {
	status := 0.0
	if active {
		status = 1.0
	}

	m.consumerStatus.WithLabelValues(
		fmt.Sprintf("%d", tenantID),
		queueName,
	).Set(status)
}

// ResetMetrics resets all metrics for a specific tenant and queue
func (m *Metrics) ResetMetrics(tenantID int, queueName string) {
	labels := prometheus.Labels{
		"tenant_id":  fmt.Sprintf("%d", tenantID),
		"queue_name": queueName,
	}

	m.queueDepth.Delete(labels)
	m.activeWorkers.Delete(labels)
	m.consumerStatus.Delete(labels)

	// Note: Counters and histograms are typically not reset in Prometheus
	// as they represent cumulative values
}
