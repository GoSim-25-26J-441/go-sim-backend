package costcal

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CostResult struct {
	Provider        string  `json:"provider"`
	FoundMatches    bool    `json:"found_matches"`
	MatchCount      int     `json:"match_count"`
	ChosenSKU       string  `json:"chosen_sku,omitempty"`
	InstanceType    string  `json:"instance_type,omitempty"`
	MatchedVCPU     int     `json:"matched_vcpu,omitempty"`
	MatchedMemoryGB float64 `json:"matched_memory_gb,omitempty"`
	Region          string  `json:"region,omitempty"`
	PricePerHour    float64 `json:"price_per_hour,omitempty"`
	MonthlyCost     float64 `json:"monthly_cost,omitempty"`
	Currency        string  `json:"currency,omitempty"`
	Unit            string  `json:"unit,omitempty"`
	Note            string  `json:"note,omitempty"`
	MatchedDistance float64 `json:"matched_distance,omitempty"`
	FetchedAt       string  `json:"fetched_at,omitempty"`
}

// HoursPerMonth returns the number of hours in a 30-day month.
func HoursPerMonth() float64 { return 24.0 * 30.0 }

// CalculateCostsForBestCandidate looks up the best match for costs, either exact or nearest match.
func CalculateCostsForBestCandidate(ctx context.Context, pool *pgxpool.Pool, best rules.CandidateScore) (map[string]CostResult, error) {
	results := make(map[string]CostResult)

	vcpu := best.Candidate.Spec.VCPU
	mem := best.Candidate.Spec.MemoryGB

	// List of providers to check
	providers := []struct {
		Name  string
		Table string
	}{
		{"aws", "aws_compute_prices"},
		{"azure", "azure_compute_prices"},
		{"gcp", "gcp_compute_prices"},
	}

	// Loop through each provider and calculate costs
	for _, p := range providers {
		cr, err := calcForProvider(ctx, pool, p.Name, p.Table, vcpu, mem)
		if err != nil {
			results[p.Name] = CostResult{
				Provider: p.Name,
				Note:     fmt.Sprintf("lookup error: %v", err),
			}
			continue
		}
		results[p.Name] = cr
	}

	return results, nil
}

// calcForProvider finds the exact or nearest match for a provider's pricing.
func calcForProvider(ctx context.Context, pool *pgxpool.Pool, provider, table string, vcpu int, mem float64) (CostResult, error) {
	cr := CostResult{Provider: provider}

	// 1) Exact match query (vcpu + memory)
	queryExact := fmt.Sprintf(`
SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as fetched_at
FROM %s
WHERE vcpu = $1 AND memory_gb = $2
  AND price_per_hour IS NOT NULL
`, table)

	rows, err := pool.Query(ctx, queryExact, vcpu, mem)
	if err != nil {
		return cr, fmt.Errorf("exact query failed: %w", err)
	}
	defer rows.Close()

	type rowT struct {
		SKUID        string
		InstanceType string
		Region       string
		VCPU         int
		MemoryGB     float64
		PriceHour    *float64
		Currency     *string
		Unit         *string
		FetchedAt    *string
	}

	matches := make([]rowT, 0)
	for rows.Next() {
		var r rowT
		if err := rows.Scan(&r.SKUID, &r.InstanceType, &r.Region, &r.VCPU, &r.MemoryGB, &r.PriceHour, &r.Currency, &r.Unit, &r.FetchedAt); err != nil {
			return cr, fmt.Errorf("scan exact row failed: %w", err)
		}
		matches = append(matches, r)
	}
	if rows.Err() != nil {
		return cr, fmt.Errorf("rows error: %w", rows.Err())
	}

	// If matches found, pick the lowest price among them
	if len(matches) > 0 {
		bestIdx := -1
		bestPrice := math.MaxFloat64
		for i, m := range matches {
			if m.PriceHour != nil && *m.PriceHour > 0 {
				if *m.PriceHour < bestPrice {
					bestPrice = *m.PriceHour
					bestIdx = i
				}
			}
		}
		if bestIdx >= 0 {
			m := matches[bestIdx]
			cr.FoundMatches = true
			cr.MatchCount = len(matches)
			cr.ChosenSKU = m.SKUID
			cr.InstanceType = m.InstanceType
			cr.MatchedVCPU = m.VCPU
			cr.MatchedMemoryGB = m.MemoryGB
			cr.Region = m.Region
			cr.PricePerHour = bestPrice
			if m.Currency != nil {
				cr.Currency = *m.Currency
			}
			if m.Unit != nil {
				cr.Unit = *m.Unit
			}
			if m.FetchedAt != nil {
				cr.FetchedAt = *m.FetchedAt
			}
			cr.MonthlyCost = cr.PricePerHour * HoursPerMonth()
			cr.Note = "exact-match: chose lowest price among exact matches"
			return cr, nil
		}
		cr.Note = "matches present but no valid price_per_hour"
	}

	// 2) Nearest match (vcpu + memory)
	queryNearest := fmt.Sprintf(`
SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as fetched_at,
       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
FROM %s
WHERE price_per_hour IS NOT NULL
ORDER BY dist ASC
LIMIT 1
`, table)

	var skuID, instanceType, region, currency, unit, fetchedAt string
	var dbVcpu int
	var dbMem float64
	var pricePerHour *float64
	var dist *float64

	err = pool.QueryRow(ctx, queryNearest, vcpu, mem).Scan(&skuID, &instanceType, &region, &dbVcpu, &dbMem, &pricePerHour, &currency, &unit, &fetchedAt, &dist)
	if err == nil {
		if pricePerHour != nil && *pricePerHour > 0 {
			cr.FoundMatches = true
			cr.MatchCount = 1
			cr.ChosenSKU = skuID
			cr.InstanceType = instanceType
			cr.MatchedVCPU = dbVcpu
			cr.MatchedMemoryGB = dbMem
			cr.Region = region
			cr.PricePerHour = *pricePerHour
			cr.MonthlyCost = cr.PricePerHour * HoursPerMonth()
			cr.Currency = currency
			cr.Unit = unit
			cr.FetchedAt = fetchedAt
			if dist != nil {
				cr.MatchedDistance = *dist
			} else {
				cr.MatchedDistance = math.Abs(float64(dbVcpu-vcpu)) + math.Abs(dbMem-mem)
			}
			cr.Note = "nearest-match: used closest vcpu+memory SKU"
			return cr, nil
		}
		cr.Note = "nearest row found but price invalid"
	}

	cr.FoundMatches = false
	cr.MatchCount = 0
	cr.Note = "no matching price rows found"
	return cr, nil
}

// CandidateScoreFromJSONBytes converts JSON bytes to a CandidateScore.
func CandidateScoreFromJSONBytes(b []byte) (rules.CandidateScore, error) {
	var cs rules.CandidateScore
	if err := json.Unmarshal(b, &cs); err != nil {
		return cs, err
	}
	return cs, nil
}
