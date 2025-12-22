package costcal

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PurchaseOption struct {
	Type                string  `json:"type"`
	LeaseContractLength string  `json:"lease_contract_length,omitempty"`
	PricePerHour        float64 `json:"price_per_hour"`
	MonthlyCost         float64 `json:"monthly_cost"`
	Currency            string  `json:"currency"`
	Unit                string  `json:"unit"`
	SavingPct           float64 `json:"saving_pct,omitempty"`
	Note                string  `json:"note,omitempty"`
}

type CostResult struct {
	Provider        string           `json:"provider"`
	FoundMatches    bool             `json:"found_matches"`
	MatchCount      int              `json:"match_count"`
	ChosenSKU       string           `json:"chosen_sku,omitempty"`
	InstanceType    string           `json:"instance_type,omitempty"`
	MatchedVCPU     int              `json:"matched_vcpu,omitempty"`
	MatchedMemoryGB float64          `json:"matched_memory_gb,omitempty"`
	Region          string           `json:"region,omitempty"`
	PurchaseOptions []PurchaseOption `json:"purchase_options"`
	FetchedAt       string           `json:"fetched_at,omitempty"`
	Note            string           `json:"note,omitempty"`
	MatchedDistance float64          `json:"matched_distance,omitempty"`
}

// HoursPerMonth returns the number of hours in a 30-day month.
func HoursPerMonth() float64 { return 24.0 * 30.0 }

// CalculateCostsForBestCandidate looks up the best match for costs with all purchase options.
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

// calcForProvider finds the exact or nearest match for a provider's pricing with all purchase options.
func calcForProvider(ctx context.Context, pool *pgxpool.Pool, provider, table string, vcpu int, mem float64) (CostResult, error) {
	cr := CostResult{Provider: provider}

	// 1) First try to find exact match
	exactMatches, err := findExactMatches(ctx, pool, provider, table, vcpu, mem)
	if err != nil {
		return cr, fmt.Errorf("finding exact matches failed: %w", err)
	}

	if len(exactMatches) > 0 {
		// Use the exact match with On-Demand pricing as reference
		return processExactMatches(exactMatches, provider, vcpu, mem), nil
	}

	// 2) If no exact matches, find nearest match
	nearestMatches, err := findNearestMatches(ctx, pool, provider, table, vcpu, mem)
	if err != nil {
		return cr, fmt.Errorf("finding nearest matches failed: %w", err)
	}

	if len(nearestMatches) > 0 {
		return processNearestMatches(nearestMatches, provider, vcpu, mem), nil
	}

	cr.FoundMatches = false
	cr.MatchCount = 0
	cr.Note = "no matching price rows found"
	return cr, nil
}

type priceRow struct {
	SKUID               string
	InstanceType        string
	Region              string
	VCPU                int
	MemoryGB            float64
	PriceHour           *float64
	Currency            *string
	Unit                *string
	FetchedAt           *string
	PurchaseOption      *string
	LeaseContractLength *string
	ServiceFamily       *string
	UsageType           *string
	Distance            float64
}

func findExactMatches(ctx context.Context, pool *pgxpool.Pool, provider, table string, vcpu int, mem float64) ([]priceRow, error) {
	query := ""

	switch provider {
	case "aws":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as fetched_at,
			       purchase_option, lease_contract_length, instance_family as service_family, NULL as usage_type
			FROM aws_compute_prices
			WHERE vcpu = $1 AND memory_gb = $2 AND price_per_hour IS NOT NULL
			ORDER BY price_per_hour ASC
		`
	case "azure":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as fetched_at,
			       purchase_option, lease_contract_length, service_family, NULL as usage_type
			FROM azure_compute_prices
			WHERE vcpu = $1 AND memory_gb = $2 AND price_per_hour IS NOT NULL
			ORDER BY price_per_hour ASC
		`
	case "gcp":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as fetched_at,
			       purchase_option, NULL as lease_contract_length, resource_family as service_family, usage_type
			FROM gcp_compute_prices
			WHERE vcpu = $1 AND memory_gb = $2 AND price_per_hour IS NOT NULL
			ORDER BY price_per_hour ASC
		`
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}

	rows, err := pool.Query(ctx, query, vcpu, mem)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var matches []priceRow
	for rows.Next() {
		var r priceRow
		err := rows.Scan(&r.SKUID, &r.InstanceType, &r.Region, &r.VCPU, &r.MemoryGB, &r.PriceHour,
			&r.Currency, &r.Unit, &r.FetchedAt, &r.PurchaseOption, &r.LeaseContractLength,
			&r.ServiceFamily, &r.UsageType)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		r.Distance = 0 // exact match
		matches = append(matches, r)
	}

	return matches, nil
}

func findNearestMatches(ctx context.Context, pool *pgxpool.Pool, provider, table string, vcpu int, mem float64) ([]priceRow, error) {
	query := ""

	switch provider {
	case "aws":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as fetched_at,
			       purchase_option, lease_contract_length, instance_family as service_family, NULL as usage_type,
			       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
			FROM aws_compute_prices
			WHERE price_per_hour IS NOT NULL
			ORDER BY dist ASC, price_per_hour ASC
			LIMIT 20
		`
	case "azure":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as fetched_at,
			       purchase_option, lease_contract_length, service_family, NULL as usage_type,
			       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
			FROM azure_compute_prices
			WHERE price_per_hour IS NOT NULL
			ORDER BY dist ASC, price_per_hour ASC
			LIMIT 20
		`
	case "gcp":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as fetched_at,
			       purchase_option, NULL as lease_contract_length, resource_family as service_family, usage_type,
			       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
			FROM gcp_compute_prices
			WHERE price_per_hour IS NOT NULL
			ORDER BY dist ASC, price_per_hour ASC
			LIMIT 20
		`
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}

	rows, err := pool.Query(ctx, query, vcpu, mem)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var matches []priceRow
	for rows.Next() {
		var r priceRow
		err := rows.Scan(&r.SKUID, &r.InstanceType, &r.Region, &r.VCPU, &r.MemoryGB, &r.PriceHour,
			&r.Currency, &r.Unit, &r.FetchedAt, &r.PurchaseOption, &r.LeaseContractLength,
			&r.ServiceFamily, &r.UsageType, &r.Distance)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		matches = append(matches, r)
	}

	return matches, nil
}

func processExactMatches(matches []priceRow, provider string, targetVCPU int, targetMem float64) CostResult {
	cr := CostResult{
		Provider:     provider,
		FoundMatches: true,
		MatchCount:   len(matches),
		Note:         "exact-match",
	}

	// Group by instance type to find the best instance for each purchase option
	instanceGroups := make(map[string][]priceRow)
	for _, match := range matches {
		if match.InstanceType != "" {
			instanceGroups[match.InstanceType] = append(instanceGroups[match.InstanceType], match)
		}
	}

	// Find the instance with the most purchase options
	var bestInstance string
	maxOptions := 0
	for instanceType, rows := range instanceGroups {
		if len(rows) > maxOptions {
			maxOptions = len(rows)
			bestInstance = instanceType
		}
	}

	if bestInstance == "" && len(matches) > 0 {
		bestInstance = matches[0].InstanceType
	}

	// Collect all rows for the best instance
	var instanceRows []priceRow
	for _, match := range matches {
		if match.InstanceType == bestInstance {
			instanceRows = append(instanceRows, match)
		}
	}

	if len(instanceRows) == 0 && len(matches) > 0 {
		instanceRows = matches
	}

	// Use the first row for metadata
	firstRow := instanceRows[0]
	cr.ChosenSKU = firstRow.SKUID
	cr.InstanceType = firstRow.InstanceType
	cr.MatchedVCPU = firstRow.VCPU
	cr.MatchedMemoryGB = firstRow.MemoryGB
	cr.Region = firstRow.Region
	if firstRow.FetchedAt != nil {
		cr.FetchedAt = *firstRow.FetchedAt
	}

	// Group purchase options
	purchaseOptions := groupPurchaseOptions(instanceRows, provider)

	// Sort purchase options: On-Demand first, then Reserved
	sort.Slice(purchaseOptions, func(i, j int) bool {
		if purchaseOptions[i].Type == "OnDemand" && purchaseOptions[j].Type != "OnDemand" {
			return true
		}
		if purchaseOptions[i].Type != "OnDemand" && purchaseOptions[j].Type == "OnDemand" {
			return false
		}
		if purchaseOptions[i].Type == "Reserved" && purchaseOptions[j].Type == "Reserved" {
			// Sort by lease length (1yr before 3yr)
			if purchaseOptions[i].LeaseContractLength == "1yr" && purchaseOptions[j].LeaseContractLength == "3yr" {
				return true
			}
			if purchaseOptions[i].LeaseContractLength == "3yr" && purchaseOptions[j].LeaseContractLength == "1yr" {
				return false
			}
			return purchaseOptions[i].LeaseContractLength < purchaseOptions[j].LeaseContractLength
		}
		return purchaseOptions[i].Type < purchaseOptions[j].Type
	})

	cr.PurchaseOptions = purchaseOptions

	return cr
}

func processNearestMatches(matches []priceRow, provider string, targetVCPU int, targetMem float64) CostResult {
	cr := CostResult{
		Provider:        provider,
		FoundMatches:    true,
		MatchCount:      len(matches),
		Note:            fmt.Sprintf("nearest-match (distance: %.2f)", matches[0].Distance),
		MatchedDistance: matches[0].Distance,
	}

	// Group by instance type
	instanceGroups := make(map[string][]priceRow)
	for _, match := range matches {
		if match.InstanceType != "" {
			instanceGroups[match.InstanceType] = append(instanceGroups[match.InstanceType], match)
		}
	}

	// Find the instance with the lowest price for On-Demand
	var bestInstance string
	lowestPrice := math.MaxFloat64

	for instanceType, rows := range instanceGroups {
		for _, row := range rows {
			if row.PriceHour != nil && *row.PriceHour < lowestPrice {
				// Check if this is an On-Demand or equivalent option
				isOnDemand := false
				if row.PurchaseOption != nil {
					po := strings.ToLower(*row.PurchaseOption)
					isOnDemand = strings.Contains(po, "ondemand") ||
						strings.Contains(po, "payg") ||
						strings.Contains(po, "consumption") ||
						po == "on_demand" ||
						strings.Contains(po, "on demand") ||
						po == "" // Sometimes empty means On-Demand
				} else {
					// If no purchase option specified, assume On-Demand
					isOnDemand = true
				}

				if isOnDemand {
					lowestPrice = *row.PriceHour
					bestInstance = instanceType
				}
			}
		}
	}

	// If no On-Demand found, use the first instance
	if bestInstance == "" && len(matches) > 0 {
		bestInstance = matches[0].InstanceType
	}

	// Collect all rows for the best instance
	var instanceRows []priceRow
	for _, match := range matches {
		if match.InstanceType == bestInstance {
			instanceRows = append(instanceRows, match)
		}
	}

	if len(instanceRows) == 0 && len(matches) > 0 {
		instanceRows = matches
	}

	// Use the first row for metadata
	firstRow := instanceRows[0]
	cr.ChosenSKU = firstRow.SKUID
	cr.InstanceType = firstRow.InstanceType
	cr.MatchedVCPU = firstRow.VCPU
	cr.MatchedMemoryGB = firstRow.MemoryGB
	cr.Region = firstRow.Region
	if firstRow.FetchedAt != nil {
		cr.FetchedAt = *firstRow.FetchedAt
	}

	// Group purchase options
	purchaseOptions := groupPurchaseOptions(instanceRows, provider)

	// Sort purchase options
	sort.Slice(purchaseOptions, func(i, j int) bool {
		if purchaseOptions[i].Type == "OnDemand" && purchaseOptions[j].Type != "OnDemand" {
			return true
		}
		if purchaseOptions[i].Type != "OnDemand" && purchaseOptions[j].Type == "OnDemand" {
			return false
		}
		if purchaseOptions[i].Type == "Reserved" && purchaseOptions[j].Type == "Reserved" {
			// Sort by lease length (1yr before 3yr)
			if purchaseOptions[i].LeaseContractLength == "1yr" && purchaseOptions[j].LeaseContractLength == "3yr" {
				return true
			}
			if purchaseOptions[i].LeaseContractLength == "3yr" && purchaseOptions[j].LeaseContractLength == "1yr" {
				return false
			}
			return purchaseOptions[i].LeaseContractLength < purchaseOptions[j].LeaseContractLength
		}
		return purchaseOptions[i].Type < purchaseOptions[j].Type
	})

	cr.PurchaseOptions = purchaseOptions

	return cr
}

func isTotalTermCost(pricePerHour float64, unit, leaseLength string) bool {

	if pricePerHour > 100 {
		return true
	}

	// Check unit for indicators of total term cost
	unitLower := strings.ToLower(unit)
	if strings.Contains(unitLower, "quantity") ||
		strings.Contains(unitLower, "upfront") ||
		strings.Contains(unitLower, "total") ||
		strings.Contains(unitLower, "term") ||
		strings.Contains(unitLower, "year") ||
		strings.Contains(unitLower, "month") && !strings.Contains(unitLower, "hour") {
		return true
	}

	return false
}

func convertTotalTermToMonthly(price float64, leaseLength string) (pricePerHour, monthlyCost float64) {
	leaseLower := strings.ToLower(leaseLength)

	// Convert total term cost to monthly equivalent
	if strings.Contains(leaseLower, "3") {
		// 3 year term (36 months)
		monthlyCost = price / 36
	} else {
		// Default to 1 year term (12 months)
		monthlyCost = price / 12
	}

	// Calculate hourly rate from monthly cost
	pricePerHour = monthlyCost / HoursPerMonth()

	return pricePerHour, monthlyCost
}

func groupPurchaseOptions(rows []priceRow, provider string) []PurchaseOption {
	purchaseOptions := make([]PurchaseOption, 0)
	optionMap := make(map[string]PurchaseOption)

	var onDemandPrice float64

	// First pass: collect all options and find On-Demand price
	for _, row := range rows {
		if row.PriceHour == nil || *row.PriceHour <= 0 {
			continue
		}

		optionType := "Other"
		leaseLength := ""

		if row.PurchaseOption != nil {
			po := strings.ToLower(*row.PurchaseOption)

			switch {
			case strings.Contains(po, "ondemand") || strings.Contains(po, "payg") ||
				strings.Contains(po, "consumption") || po == "on_demand" ||
				strings.Contains(po, "on demand") || po == "":
				optionType = "OnDemand"
				onDemandPrice = *row.PriceHour

			case strings.Contains(po, "reserved") || strings.Contains(po, "savings") ||
				strings.Contains(po, "commitment") || strings.Contains(po, "reservation"):
				optionType = "Reserved"

				// Parse lease contract length
				if row.LeaseContractLength != nil {
					leaseLength = *row.LeaseContractLength
				}

				// If no lease length in database, try to infer from purchase option
				if leaseLength == "" {
					if strings.Contains(po, "1year") || strings.Contains(po, "1-year") ||
						strings.Contains(po, "1 yr") || strings.Contains(po, "1y") ||
						strings.Contains(po, "12 month") {
						leaseLength = "1yr"
					} else if strings.Contains(po, "3year") || strings.Contains(po, "3-year") ||
						strings.Contains(po, "3 yr") || strings.Contains(po, "3y") ||
						strings.Contains(po, "36 month") {
						leaseLength = "3yr"
					} else {
						leaseLength = "1yr"
					}
				}
			}
		} else {
			// If no purchase option specified, assume On-Demand
			optionType = "OnDemand"
			onDemandPrice = *row.PriceHour
		}

		key := fmt.Sprintf("%s_%s", optionType, leaseLength)

		currency := "USD"
		if row.Currency != nil {
			currency = *row.Currency
		}

		unit := "USD"
		if row.Unit != nil {
			unit = *row.Unit
		}

		pricePerHour := *row.PriceHour
		monthlyCost := pricePerHour * HoursPerMonth()

		// For Reserved instances, check if price represents total term cost
		if optionType == "Reserved" {
			// Check multiple indicators that this might be total term cost
			if isTotalTermCost(pricePerHour, unit, leaseLength) {
				// Convert total term cost to monthly equivalent
				pricePerHour, monthlyCost = convertTotalTermToMonthly(pricePerHour, leaseLength)
			}
		}

		// Round the values
		pricePerHour = math.Round(pricePerHour*100000) / 100000
		monthlyCost = math.Round(monthlyCost*100) / 100

		optionMap[key] = PurchaseOption{
			Type:                optionType,
			LeaseContractLength: leaseLength,
			PricePerHour:        pricePerHour,
			MonthlyCost:         monthlyCost,
			Currency:            currency,
			Unit:                unit,
		}
	}

	//  calculate savings percentages for Reserved instances
	for key, option := range optionMap {
		if option.Type == "Reserved" && onDemandPrice > 0 {

			if option.PricePerHour > 0 && option.PricePerHour < onDemandPrice {
				savings := ((onDemandPrice - option.PricePerHour) / onDemandPrice) * 100
				if savings > 100 {
					savings = 100
				} else if savings < 0 {
					savings = 0
				}
				option.SavingPct = math.Round(savings*10) / 10
			} else if option.PricePerHour > 0 && option.PricePerHour < onDemandPrice*2 {

				savings := ((onDemandPrice - option.PricePerHour) / onDemandPrice) * 100
				if savings > 0 {
					option.SavingPct = math.Round(savings*10) / 10
				} else {
					option.SavingPct = 0
				}
			} else {
				option.SavingPct = 0
				if option.MonthlyCost < option.PricePerHour*HoursPerMonth()*0.1 {

					option.Note = "Converted from total term cost"
				}
			}
			optionMap[key] = option
		}

		purchaseOptions = append(purchaseOptions, optionMap[key])
	}

	return purchaseOptions
}

// CandidateScoreFromJSONBytes converts JSON bytes to a CandidateScore.
func CandidateScoreFromJSONBytes(b []byte) (rules.CandidateScore, error) {
	var cs rules.CandidateScore
	if err := json.Unmarshal(b, &cs); err != nil {
		return cs, err
	}
	return cs, nil
}
