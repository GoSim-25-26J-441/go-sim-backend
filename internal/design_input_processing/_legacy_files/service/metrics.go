package service

import (
	"sync/atomic"
	"time"
)

// Metrics tracks service call metrics
type Metrics struct {
	upstreamCalls      int64
	upstreamErrors     int64
	upstreamLatency    int64 // Total latency in nanoseconds
	jobServiceCalls    int64
	graphServiceCalls  int64
	signalServiceCalls int64
}

var globalMetrics = &Metrics{}

// GetMetrics returns the current metrics snapshot
func GetMetrics() Metrics {
	return Metrics{
		upstreamCalls:      atomic.LoadInt64(&globalMetrics.upstreamCalls),
		upstreamErrors:     atomic.LoadInt64(&globalMetrics.upstreamErrors),
		upstreamLatency:    atomic.LoadInt64(&globalMetrics.upstreamLatency),
		jobServiceCalls:    atomic.LoadInt64(&globalMetrics.jobServiceCalls),
		graphServiceCalls:  atomic.LoadInt64(&globalMetrics.graphServiceCalls),
		signalServiceCalls: atomic.LoadInt64(&globalMetrics.signalServiceCalls),
	}
}

// ResetMetrics resets all metrics (useful for testing)
func ResetMetrics() {
	atomic.StoreInt64(&globalMetrics.upstreamCalls, 0)
	atomic.StoreInt64(&globalMetrics.upstreamErrors, 0)
	atomic.StoreInt64(&globalMetrics.upstreamLatency, 0)
	atomic.StoreInt64(&globalMetrics.jobServiceCalls, 0)
	atomic.StoreInt64(&globalMetrics.graphServiceCalls, 0)
	atomic.StoreInt64(&globalMetrics.signalServiceCalls, 0)
}

// recordUpstreamCall records an upstream service call
func recordUpstreamCall(duration time.Duration, err error) {
	atomic.AddInt64(&globalMetrics.upstreamCalls, 1)
	atomic.AddInt64(&globalMetrics.upstreamLatency, duration.Nanoseconds())
	if err != nil {
		atomic.AddInt64(&globalMetrics.upstreamErrors, 1)
	}
}

// recordJobServiceCall records a job service call
func recordJobServiceCall() {
	atomic.AddInt64(&globalMetrics.jobServiceCalls, 1)
}

// recordGraphServiceCall records a graph service call
func recordGraphServiceCall() {
	atomic.AddInt64(&globalMetrics.graphServiceCalls, 1)
}

// recordSignalServiceCall records a signal service call
func recordSignalServiceCall() {
	atomic.AddInt64(&globalMetrics.signalServiceCalls, 1)
}

// AverageUpstreamLatency returns the average latency in milliseconds
func (m Metrics) AverageUpstreamLatency() float64 {
	if m.upstreamCalls == 0 {
		return 0
	}
	avgNs := float64(m.upstreamLatency) / float64(m.upstreamCalls)
	return avgNs / 1e6 // Convert nanoseconds to milliseconds
}

// UpstreamErrorRate returns the error rate as a percentage
func (m Metrics) UpstreamErrorRate() float64 {
	if m.upstreamCalls == 0 {
		return 0
	}
	return float64(m.upstreamErrors) / float64(m.upstreamCalls) * 100
}
