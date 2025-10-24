package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultBatchSize = 500

func main() {
	dir := flag.String("dir", "out", "directory containing provider CSV files (e.g. out/asm for Azure)")
	batch := flag.Int("batch", defaultBatchSize, "batch size for inserts")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	azureFile := filepathJoin(*dir, "asm", "azure_compute_prices.csv")
	gcpFile := filepathJoin(*dir, "asm", "gcp_compute_prices.csv")
	awsFile := filepathJoin(*dir, "asm", "aws_compute_prices.csv")

	if exists(azureFile) {
		log.Printf("Importing Azure CSV: %s", azureFile)
		if err := importAzureCSV(ctx, pool, azureFile, *batch); err != nil {
			log.Fatalf("Azure import failed: %v", err)
		}
	} else {
		log.Printf("Azure CSV not found: %s (skipping)", azureFile)
	}

	if exists(gcpFile) {
		log.Printf("Importing GCP CSV: %s", gcpFile)
		if err := importGCPCSV(ctx, pool, gcpFile, *batch); err != nil {
			log.Fatalf("GCP import failed: %v", err)
		}
	} else {
		log.Printf("GCP CSV not found: %s (skipping)", gcpFile)
	}

	if exists(awsFile) {
		log.Printf("Importing AWS CSV: %s", awsFile)
		if err := importAWSCSV(ctx, pool, awsFile, *batch); err != nil {
			log.Fatalf("AWS import failed: %v", err)
		}
	} else {
		log.Printf("AWS CSV not found: %s (skipping)", awsFile)
	}

	log.Println("✅ All imports finished successfully.")
}

func filepathJoin(parts ...string) string {
	return strings.Join(parts, string(os.PathSeparator))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ---------- Azure importer ----------
func importAzureCSV(ctx context.Context, pool *pgxpool.Pool, path string, batchSize int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	idx := mapHeaderIndices(header, []string{
		"sku_id", "region", "instance_type", "vcpu", "memory_gb", "price_per_hour", "currency", "unit", "fetched_at",
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	count := 0
	batch := make([][]interface{}, 0, batchSize)
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("csv read error: %w", err)
		}
		row := []interface{}{
			strings.TrimSpace(rec[idx["sku_id"]]),
			"azure",
			strings.TrimSpace(rec[idx["region"]]),
			strings.TrimSpace(rec[idx["instance_type"]]),
			parseNullableInt(rec[idx["vcpu"]]),
			parseNullableFloat(rec[idx["memory_gb"]]),
			parseNullableFloat(rec[idx["price_per_hour"]]),
			strings.TrimSpace(rec[idx["currency"]]),
			strings.TrimSpace(rec[idx["unit"]]),
			parseTimeOrNow(rec[idx["fetched_at"]]),
			"{}",
		}
		batch = append(batch, row)
		count++
		if len(batch) >= batchSize {
			if err := flushBatch(ctx, tx, "azure_compute_prices", batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := flushBatch(ctx, tx, "azure_compute_prices", batch); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("Azure import complete — %d rows processed", count)
	return nil
}

// ---------- GCP importer ----------
func importGCPCSV(ctx context.Context, pool *pgxpool.Pool, path string, batchSize int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return err
	}

	idx := mapHeaderIndices(header, []string{
		"sku_id", "region", "description", "vcpu", "memory_gb", "price_per_hour", "currency", "unit", "fetched_at",
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	count := 0
	batch := make([][]interface{}, 0, batchSize)
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("csv read error: %w", err)
		}
		row := []interface{}{
			strings.TrimSpace(rec[idx["sku_id"]]),
			"gcp",
			strings.TrimSpace(rec[idx["region"]]),
			strings.TrimSpace(rec[idx["description"]]),
			parseNullableInt(rec[idx["vcpu"]]),
			parseNullableFloat(rec[idx["memory_gb"]]),
			parseNullableFloat(rec[idx["price_per_hour"]]),
			strings.TrimSpace(rec[idx["currency"]]),
			strings.TrimSpace(rec[idx["unit"]]),
			parseTimeOrNow(rec[idx["fetched_at"]]),
			"{}",
		}
		batch = append(batch, row)
		count++
		if len(batch) >= batchSize {
			if err := flushBatch(ctx, tx, "gcp_compute_prices", batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := flushBatch(ctx, tx, "gcp_compute_prices", batch); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("GCP import complete — %d rows processed", count)
	return nil
}

// ---------- AWS importer ----------
func importAWSCSV(ctx context.Context, pool *pgxpool.Pool, path string, batchSize int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return err
	}

	idx := mapHeaderIndices(header, []string{
		"sku_id", "region", "instance_type", "vcpu", "memory_gb", "price_per_hour", "currency", "unit", "fetched_at",
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	count := 0
	batch := make([][]interface{}, 0, batchSize)
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("csv read error: %w", err)
		}
		row := []interface{}{
			strings.TrimSpace(rec[idx["sku_id"]]),
			"aws",
			strings.TrimSpace(rec[idx["region"]]),
			strings.TrimSpace(rec[idx["instance_type"]]),
			parseNullableInt(rec[idx["vcpu"]]),
			parseNullableFloat(rec[idx["memory_gb"]]),
			parseNullableFloat(rec[idx["price_per_hour"]]),
			strings.TrimSpace(rec[idx["currency"]]),
			strings.TrimSpace(rec[idx["unit"]]),
			parseTimeOrNow(rec[idx["fetched_at"]]),
			"{}",
		}
		batch = append(batch, row)
		count++
		if len(batch) >= batchSize {
			if err := flushBatch(ctx, tx, "aws_compute_prices", batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := flushBatch(ctx, tx, "aws_compute_prices", batch); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("AWS import complete — %d rows processed", count)
	return nil
}

// ---------- Shared helpers ----------
func flushBatch(ctx context.Context, tx pgx.Tx, table string, rows [][]interface{}) error {
	n := len(rows)
	if n == 0 {
		return nil
	}

	dedup := make(map[string][]interface{}, n)
	keysOrder := make([]string, 0, n)
	for _, r := range rows {
		sku := fmt.Sprintf("%v", r[0])
		region := fmt.Sprintf("%v", r[2])
		key := sku + "|" + region
		if _, ok := dedup[key]; !ok {
			keysOrder = append(keysOrder, key)
		}
		dedup[key] = r
	}

	dedupRows := make([][]interface{}, 0, len(dedup))
	for _, key := range keysOrder {
		dedupRows = append(dedupRows, dedup[key])
	}

	m := len(dedupRows)
	valuePlaceholders := make([]string, 0, m)
	args := make([]interface{}, 0, m*11)
	argPos := 1
	for _, r := range dedupRows {
		valuePlaceholders = append(valuePlaceholders,
			fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
				argPos, argPos+1, argPos+2, argPos+3, argPos+4, argPos+5,
				argPos+6, argPos+7, argPos+8, argPos+9, argPos+10))
		args = append(args, r...)
		argPos += 11
	}

	sql := fmt.Sprintf(`
INSERT INTO %s
  (sku_id, provider, region, instance_type, vcpu, memory_gb, price_per_hour, currency, unit, fetched_at, metadata)
VALUES %s
ON CONFLICT (sku_id, region) DO UPDATE
  SET instance_type = EXCLUDED.instance_type,
      vcpu = EXCLUDED.vcpu,
      memory_gb = EXCLUDED.memory_gb,
      price_per_hour = EXCLUDED.price_per_hour,
      currency = EXCLUDED.currency,
      unit = EXCLUDED.unit,
      fetched_at = EXCLUDED.fetched_at,
      metadata = EXCLUDED.metadata,
      updated_at = now();
`, table, strings.Join(valuePlaceholders, ","))

	_, err := tx.Exec(ctx, sql, args...)
	return err
}

func mapHeaderIndices(header []string, want []string) map[string]int {
	idx := map[string]int{}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		idx[h] = i
	}
	for _, w := range want {
		if _, ok := idx[w]; !ok {
			log.Fatalf("expected header %q not found in file", w)
		}
	}
	return idx
}

func parseNullableInt(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return i
}

func parseNullableFloat(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return f
}

func parseTimeOrNow(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		if t2, err2 := time.Parse(time.RFC3339, s); err2 == nil {
			return t2.UTC()
		}
		return time.Now().UTC()
	}
	return t.UTC()
}
