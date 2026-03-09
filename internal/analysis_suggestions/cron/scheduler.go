package cronjob

import (
	"context"
	"log"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/fetchers"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/importer"
	"github.com/robfig/cron/v3"
)

type Scheduler struct{}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

// Start initializes cron tasks
func (s *Scheduler) Start() {
	c := cron.New(cron.WithSeconds())

	//  (12:00 AM)
	_, err := c.AddFunc("0 0 0 * * *", func() {
		runNightlyJobs()
	})

	if err != nil {
		log.Printf("Failed to create cron job: %v", err)
		return
	}

	log.Println("Cron scheduler started (running nightly at 12:00AM)")
	c.Start()
}

func runNightlyJobs() {
	log.Println(" Nightly job started (fetch + import)...")
	ctx := context.Background()

	if err := fetchers.RunAll(ctx, "out"); err != nil {
		log.Printf("Fetcher failed: %v", err)
		return
	}

	if err := importer.Run(ctx, "out", importer.DefaultBatchSize); err != nil {
		log.Printf("Import failed: %v", err)
		return
	}

	log.Println("Nightly job completed successfully at:", time.Now().Format(time.RFC1123))
}
