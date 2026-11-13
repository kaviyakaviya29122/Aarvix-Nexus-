package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	JobsByStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nexus_jobs_by_status", Help: "Current number of jobs per status."}, []string{"status"})
	WorkersOnline = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nexus_workers_online", Help: "Workers with a fresh heartbeat."})
	WorkersMarkedOffline = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_workers_marked_offline_total", Help: "Workers declared dead by failure detection."})
	Recoveries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_recoveries_total", Help: "Interrupted jobs recovered (requeued or dead-lettered)."})
	DuplicatesBlocked = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_duplicates_blocked_total", Help: "Duplicate job submissions rejected."})
	StaleDeliveries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nexus_stale_deliveries_total", Help: "Messages dropped because the fencing token was stale."})
	JobsFinished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nexus_jobs_finished_total", Help: "Attempts finished by outcome."}, []string{"outcome"})
	JobDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "nexus_job_duration_seconds", Help: "Execution time of job attempts.",
		Buckets: []float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120, 300}})
)
