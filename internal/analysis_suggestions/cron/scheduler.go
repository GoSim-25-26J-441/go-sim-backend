package cronjob

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

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

	wd, _ := os.Getwd()

	// Fetch prices
	fetcher := filepath.Join(wd, "internal", "analysis_suggestions", "fetchers", "fetcher_manager.go")
	cmdFetch := exec.Command("go", "run", fetcher)
	cmdFetch.Stdout = os.Stdout
	cmdFetch.Stderr = os.Stderr

	if err := cmdFetch.Run(); err != nil {
		log.Printf("Fetcher failed: %v", err)
		return
	}

	// Import into DB
	importer := filepath.Join(wd, "internal", "analysis_suggestions", "importer", "import_prices.go")
	cmdImport := exec.Command("go", "run", importer, "--dir", "out")
	cmdImport.Stdout = os.Stdout
	cmdImport.Stderr = os.Stderr

	if err := cmdImport.Run(); err != nil {
		log.Printf("Import failed: %v", err)
		return
	}

	log.Println("Nightly job completed successfully at:", time.Now().Format(time.RFC1123))
}
