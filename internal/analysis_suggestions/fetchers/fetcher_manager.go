package fetchers

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RunAll runs Azure, AWS (and optionally GCP) fetchers concurrently and writes
// CSVs under outDir/asm. It returns after all finish; errors are logged and
// the first non-nil error is returned.
func RunAll(ctx context.Context, outDir string) error {
	log.Println("Starting Fetcher Manager: running Azure, GCP, and AWS fetchers concurrently...")

	asmDir := filepath.Join(outDir, "asm")
	if err := os.MkdirAll(asmDir, 0o755); err != nil {
		return err
	}

	start := time.Now()

	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	// Azure
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("Starting Azure fetcher...")
		if err := RunAzure(ctx, outDir); err != nil {
			errChan <- err
			log.Printf("Azure fetcher failed: %v", err)
			return
		}
		log.Printf("Azure fetcher completed successfully")
	}()

	// AWS
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("Starting AWS fetcher...")
		if err := RunAWS(ctx, outDir); err != nil {
			errChan <- err
			log.Printf("AWS fetcher failed: %v", err)
			return
		}
		log.Printf("AWS fetcher completed successfully")
	}()

	// GCP (commented in original; enable if needed)
	// wg.Add(1)
	// go func() {
	// 	defer wg.Done()
	// 	log.Printf("Starting GCP fetcher...")
	// 	if err := RunGCP(ctx, outDir); err != nil {
	// 		errChan <- err
	// 		log.Printf("GCP fetcher failed: %v", err)
	// 		return
	// 	}
	// 	log.Printf("GCP fetcher completed successfully")
	// }()

	wg.Wait()
	close(errChan)

	var firstErr error
	for err := range errChan {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	log.Printf("All fetchers finished in %.2f seconds", time.Since(start).Seconds())
	return firstErr
}
