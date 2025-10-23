package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Provider struct {
	Name string
	Path string
}

func main() {
	log.Println("Starting Fetcher Manager: running Azure, GCP, and AWS fetchers concurrently...")

	outDir := "out"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("failed to create output dir: %v", err)
	}

	start := time.Now()
	wd, _ := os.Getwd()

	fetchers := []Provider{
		{
			Name: "Azure",

			Path: filepath.Join(wd, "internal", "analysis_suggestions", "fetchers", "azure_compute_fetcher.go"),
		},
		{
			Name: "GCP",
			Path: filepath.Join(wd, "internal", "analysis_suggestions", "fetchers", "gcp_compute_fetcher.go"),
		},
		{
			Name: "AWS",
			Path: filepath.Join(wd, "internal", "analysis_suggestions", "fetchers", "aws_compute_fetcher.go"),
		},
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(fetchers))

	for _, f := range fetchers {
		wg.Add(1)
		go func(f Provider) {
			defer wg.Done()
			runFetcher(f, errChan)
		}(f)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("⚠️ Fetcher error: %v", err)
		}
	}

	log.Printf("✅ All fetchers finished in %.2f seconds\n", time.Since(start).Seconds())
}

func runFetcher(f Provider, errChan chan<- error) {
	log.Printf("▶️ Starting %s fetcher...", f.Name)
	cmd := exec.Command("go", "run", f.Path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		errChan <- err
		log.Printf("❌ %s fetcher failed: %v", f.Name, err)
		return
	}
	log.Printf("✅ %s fetcher completed successfully", f.Name)
}
