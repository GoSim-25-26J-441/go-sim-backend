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
		"sku_id", "region", "instance_type", "vcpu", "memory_gb", "price_per_hour",
		"currency", "unit", "service_family", "purchase_option", "lease_contract_length", "fetched_at",
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
			strings.TrimSpace(rec[idx["sku_id"]]),                // sku_id
			"azure",                                              // provider
			strings.TrimSpace(rec[idx["region"]]),                // region
			strings.TrimSpace(rec[idx["instance_type"]]),         // instance_type
			parseNullableInt(rec[idx["vcpu"]]),                   // vcpu
			parseNullableFloat(rec[idx["memory_gb"]]),            // memory_gb
			parseNullableFloat(rec[idx["price_per_hour"]]),       // price_per_hour
			strings.TrimSpace(rec[idx["currency"]]),              // currency
			strings.TrimSpace(rec[idx["unit"]]),                  // unit
			strings.TrimSpace(rec[idx["service_family"]]),        // service_family
			strings.TrimSpace(rec[idx["purchase_option"]]),       // purchase_option
			strings.TrimSpace(rec[idx["lease_contract_length"]]), // lease_contract_length
			parseTimeOrNow(rec[idx["fetched_at"]]),               // fetched_at
			"{}",                                                 // metadata
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

// ---------- GCP importer (UPDATED) ----------
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

	// Debug: print actual headers
	log.Printf("GCP CSV headers: %v", header)

	// Map the actual headers from your GCP CSV file
	// Based on the simplified GCP fetcher output, the headers should be:
	// id, provider, sku_id, region, instance_type, resource_family, vcpu, memory_gb, price_per_hour, currency, unit, purchase_option, usage_type, fetched_at
	idx := mapHeaderIndices(header, []string{
		"sku_id", "region", "instance_type", "resource_family", "vcpu", "memory_gb",
		"price_per_hour", "currency", "unit", "purchase_option", "usage_type", "fetched_at",
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

		// Build row - using empty string for description since it's not in the CSV
		row := []interface{}{
			strings.TrimSpace(rec[idx["sku_id"]]),        // sku_id
			"gcp",                                        // provider
			strings.TrimSpace(rec[idx["region"]]),        // region
			strings.TrimSpace(rec[idx["instance_type"]]), // instance_type
			"", // description (not in CSV)
			strings.TrimSpace(rec[idx["resource_family"]]), // resource_family
			parseNullableInt(rec[idx["vcpu"]]),             // vcpu
			parseNullableFloat(rec[idx["memory_gb"]]),      // memory_gb
			parseNullableFloat(rec[idx["price_per_hour"]]), // price_per_hour
			strings.TrimSpace(rec[idx["currency"]]),        // currency
			strings.TrimSpace(rec[idx["unit"]]),            // unit
			strings.TrimSpace(rec[idx["purchase_option"]]), // purchase_option
			strings.TrimSpace(rec[idx["usage_type"]]),      // usage_type
			parseTimeOrNow(rec[idx["fetched_at"]]),         // fetched_at
			"{}",                                           // metadata
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

	// Expect the CSV to contain these headers
	idx := mapHeaderIndices(header, []string{
		"id", "sku_id", "region", "instance_type", "instancefamily",
		"vcpu", "memory_gb", "price_per_hour", "currency", "unit",
		"purchase_option", "lease_contract_length", "fetched_at",
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

		// read safely using indices
		id := strings.TrimSpace(rec[idx["id"]])
		sku := strings.TrimSpace(rec[idx["sku_id"]])
		region := strings.TrimSpace(rec[idx["region"]])
		instanceType := strings.TrimSpace(rec[idx["instance_type"]])
		instanceFamily := strings.TrimSpace(rec[idx["instancefamily"]])
		vcpu := parseNullableInt(rec[idx["vcpu"]])
		mem := parseNullableFloat(rec[idx["memory_gb"]])
		price := parseNullableFloat(rec[idx["price_per_hour"]])
		currency := strings.TrimSpace(rec[idx["currency"]])
		unit := strings.TrimSpace(rec[idx["unit"]])
		purchaseOpt := strings.TrimSpace(rec[idx["purchase_option"]])
		leaseLen := strings.TrimSpace(rec[idx["lease_contract_length"]])
		fetched := parseTimeOrNow(rec[idx["fetched_at"]])

		row := []interface{}{
			id,
			"aws",
			sku,
			region,
			instanceType,
			instanceFamily,
			vcpu,
			mem,
			price,
			currency,
			unit,
			purchaseOpt,
			leaseLen,
			fetched,
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
		var key string
		switch table {
		case "aws_compute_prices":
			sku := fmt.Sprintf("%v", r[2])
			region := fmt.Sprintf("%v", r[3])
			purchaseOpt := fmt.Sprintf("%v", r[11])
			lease := fmt.Sprintf("%v", r[12])
			key = sku + "|" + region + "|" + purchaseOpt + "|" + lease
		case "azure_compute_prices":
			sku := fmt.Sprintf("%v", r[0])
			region := fmt.Sprintf("%v", r[2])
			key = sku + "|" + region
		case "gcp_compute_prices":
			sku := fmt.Sprintf("%v", r[0])
			region := fmt.Sprintf("%v", r[2])
			key = sku + "|" + region
		default:
			sku := fmt.Sprintf("%v", r[0])
			region := fmt.Sprintf("%v", r[2])
			key = sku + "|" + region
		}
		if _, ok := dedup[key]; !ok {
			keysOrder = append(keysOrder, key)
		}
		dedup[key] = r
	}

	dedupRows := make([][]interface{}, 0, len(dedup))
	for _, key := range keysOrder {
		dedupRows = append(dedupRows, dedup[key])
	}

	var cols []string
	var conflictTarget string
	var updateSets []string

	switch table {
	case "aws_compute_prices":
		cols = []string{
			"id", "provider", "sku_id", "region", "instance_type", "instance_family",
			"vcpu", "memory_gb", "price_per_hour", "currency", "unit",
			"purchase_option", "lease_contract_length", "fetched_at",
		}
		conflictTarget = "(sku_id, region, purchase_option, lease_contract_length)"
		updateSets = []string{
			"instance_type = EXCLUDED.instance_type",
			"instance_family = EXCLUDED.instance_family",
			"vcpu = EXCLUDED.vcpu",
			"memory_gb = EXCLUDED.memory_gb",
			"price_per_hour = EXCLUDED.price_per_hour",
			"currency = EXCLUDED.currency",
			"unit = EXCLUDED.unit",
			"purchase_option = EXCLUDED.purchase_option",
			"lease_contract_length = EXCLUDED.lease_contract_length",
			"fetched_at = EXCLUDED.fetched_at",
			"updated_at = now()",
		}
	case "azure_compute_prices":
		cols = []string{
			"sku_id", "provider", "region", "instance_type", "vcpu", "memory_gb",
			"price_per_hour", "currency", "unit", "service_family", "purchase_option",
			"lease_contract_length", "fetched_at", "metadata",
		}
		conflictTarget = "(sku_id, region)"
		updateSets = []string{
			"instance_type = EXCLUDED.instance_type",
			"vcpu = EXCLUDED.vcpu",
			"memory_gb = EXCLUDED.memory_gb",
			"price_per_hour = EXCLUDED.price_per_hour",
			"currency = EXCLUDED.currency",
			"unit = EXCLUDED.unit",
			"service_family = EXCLUDED.service_family",
			"purchase_option = EXCLUDED.purchase_option",
			"lease_contract_length = EXCLUDED.lease_contract_length",
			"fetched_at = EXCLUDED.fetched_at",
			"metadata = EXCLUDED.metadata",
			"updated_at = now()",
		}
	case "gcp_compute_prices":
		// GCP columns - description is included but will be empty
		cols = []string{
			"sku_id", "provider", "region", "instance_type", "description",
			"resource_family", "vcpu", "memory_gb", "price_per_hour", "currency",
			"unit", "purchase_option", "usage_type", "fetched_at", "metadata",
		}
		conflictTarget = "(sku_id, region)"
		updateSets = []string{
			"instance_type = EXCLUDED.instance_type",
			"description = EXCLUDED.description",
			"resource_family = EXCLUDED.resource_family",
			"vcpu = EXCLUDED.vcpu",
			"memory_gb = EXCLUDED.memory_gb",
			"price_per_hour = EXCLUDED.price_per_hour",
			"currency = EXCLUDED.currency",
			"unit = EXCLUDED.unit",
			"purchase_option = EXCLUDED.purchase_option",
			"usage_type = EXCLUDED.usage_type",
			"fetched_at = EXCLUDED.fetched_at",
			"metadata = EXCLUDED.metadata",
			"updated_at = now()",
		}
	default:
		cols = []string{"sku_id", "provider", "region", "instance_type", "vcpu", "memory_gb", "price_per_hour", "currency", "unit", "fetched_at", "metadata"}
		conflictTarget = "(sku_id, region)"
		updateSets = []string{
			"instance_type = EXCLUDED.instance_type",
			"vcpu = EXCLUDED.vcpu",
			"memory_gb = EXCLUDED.memory_gb",
			"price_per_hour = EXCLUDED.price_per_hour",
			"currency = EXCLUDED.currency",
			"unit = EXCLUDED.unit",
			"fetched_at = EXCLUDED.fetched_at",
			"metadata = EXCLUDED.metadata",
			"updated_at = now()",
		}
	}

	m := len(dedupRows)
	valuePlaceholders := make([]string, 0, m)
	args := make([]interface{}, 0, m*len(cols))
	argPos := 1
	for _, r := range dedupRows {
		ph := make([]string, 0, len(cols))
		for i := 0; i < len(cols); i++ {
			ph = append(ph, fmt.Sprintf("$%d", argPos))
			args = append(args, r[i])
			argPos++
		}
		valuePlaceholders = append(valuePlaceholders, fmt.Sprintf("(%s)", strings.Join(ph, ",")))
	}

	sql := fmt.Sprintf(`
INSERT INTO %s (%s)
VALUES %s
ON CONFLICT %s DO UPDATE
  SET %s;
`, table, strings.Join(cols, ","), strings.Join(valuePlaceholders, ","), conflictTarget, strings.Join(updateSets, ",\n      "))

	_, err := tx.Exec(ctx, sql, args...)
	return err
}

func mapHeaderIndices(header []string, want []string) map[string]int {
	idx := map[string]int{}
	for i, h := range header {
		hTrim := strings.ToLower(strings.TrimSpace(h))
		idx[hTrim] = i
	}

	// Debug: print what we found vs what we want
	log.Printf("Looking for headers: %v", want)
	log.Printf("Found headers: %v", idx)

	for _, w := range want {
		wLower := strings.ToLower(w)
		if _, ok := idx[wLower]; !ok {
			log.Printf("Available headers: %v", header)
			log.Fatalf("expected header %q not found in file. Available headers: %v", w, header)
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
