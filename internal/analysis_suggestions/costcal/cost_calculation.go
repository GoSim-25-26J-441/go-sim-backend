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

type ClusterCostResult struct {
	Provider          string `json:"provider"`
	PurchaseType      string `json:"purchase_type"`
	LeaseContractType string `json:"lease_contract_length"`

	InstanceType string `json:"instance_type"`
	Region       string `json:"region"`
	Nodes        int    `json:"nodes"`

	PricePerNodeHour  float64 `json:"price_per_node_hour"`
	PricePerNodeMonth float64 `json:"price_per_node_month"`

	ControlPlaneTier  string  `json:"control_plane_tier"`
	ControlPlaneHour  float64 `json:"control_plane_hour"`
	ControlPlaneMonth float64 `json:"control_plane_month"`

	TotalHour    float64 `json:"total_hour"`
	TotalMonth   float64 `json:"total_month"`
	BudgetMonth  float64 `json:"budget_month"`
	WithinBudget bool    `json:"within_budget"`
}

func HoursPerMonth() float64 { return 24 * 30 }

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// VM MATCH
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

// Calculate costs for a specific provider and region
func CalculateCostsForProviderInRegion(ctx context.Context, pool *pgxpool.Pool, provider string, best rules.CandidateScore, region string) (CostResult, error) {
	vcpu := best.Candidate.Spec.VCPU
	mem := best.Candidate.Spec.MemoryGB

	table := map[string]string{
		"aws":   "aws_compute_prices",
		"azure": "azure_compute_prices",
		"gcp":   "gcp_compute_prices",
	}[provider]

	if table == "" {
		return CostResult{Provider: provider}, fmt.Errorf("unknown provider: %s", provider)
	}

	return calcForProviderInRegion(ctx, pool, provider, table, vcpu, mem, region)
}

// CONTROL PLANE
func GetControlPlanePrice(ctx context.Context, pool *pgxpool.Pool, provider string) (tier string, price float64, err error) {
	service := map[string]string{
		"aws":   "eks",
		"azure": "aks",
		"gcp":   "gke",
	}[provider]

	err = pool.QueryRow(ctx, `
		SELECT tier, price_per_hour
		FROM k8s_control_plane_prices
		WHERE provider = $1 AND service = $2
		ORDER BY price_per_hour ASC
		LIMIT 1
	`, provider, service).Scan(&tier, &price)

	return
}

// CLUSTER COST - Main function
func CalculateClusterCosts(
	ctx context.Context,
	pool *pgxpool.Pool,
	best rules.CandidateScore,
	nodeCount int,
	region string,
	budgetMonth float64,
	providerRegionOverride string,
) (map[string][]ClusterCostResult, error) {

	perNodeCosts, err := CalculateCostsForBestCandidate(ctx, pool, best)
	if err != nil {
		return nil, err
	}

	results := make(map[string][]ClusterCostResult)

	for _, provider := range []string{"aws", "azure", "gcp"} {
		nodeCost := perNodeCosts[provider]
		list := []ClusterCostResult{}

		cpTier, cpHour, _ := GetControlPlanePrice(ctx, pool, provider)
		cpMonth := cpHour * HoursPerMonth()

		if !nodeCost.FoundMatches {
			results[provider] = list
			continue
		}

		if provider == providerRegionOverride && region != "" {
			recalculatedCost, err := CalculateCostsForProviderInRegion(ctx, pool, provider, best, region)
			if err == nil && recalculatedCost.FoundMatches {
				nodeCost = recalculatedCost
			}
		}

		selectedRegion := nodeCost.Region

		var onDemand, res1yr, res3yr *PurchaseOption

		for _, po := range nodeCost.PurchaseOptions {
			t := strings.ToLower(po.Type)
			ll := strings.ToLower(po.LeaseContractLength)

			switch {
			case t == "ondemand" || po.Type == "":
				onDemand = &po
			case t == "reserved" && strings.Contains(ll, "1"):
				res1yr = &po
			case t == "reserved" && strings.Contains(ll, "3"):
				res3yr = &po
			}
		}

		add := func(label, lease string, po *PurchaseOption) {
			if po == nil {
				return
			}

			nHr := po.PricePerHour
			nMo := nHr * HoursPerMonth()

			totalHr := float64(nodeCount)*nHr + cpHour
			totalMo := totalHr * HoursPerMonth()

			list = append(list, ClusterCostResult{
				Provider:          provider,
				PurchaseType:      label,
				LeaseContractType: lease,
				InstanceType:      nodeCost.InstanceType,
				Region:            selectedRegion,
				Nodes:             nodeCount,

				PricePerNodeHour:  round(nHr, 5),
				PricePerNodeMonth: round(nMo, 2),

				ControlPlaneTier:  cpTier,
				ControlPlaneHour:  round(cpHour, 5),
				ControlPlaneMonth: round(cpMonth, 2),

				TotalHour:    round(totalHr, 5),
				TotalMonth:   round(totalMo, 2),
				BudgetMonth:  budgetMonth,
				WithinBudget: budgetMonth > 0 && totalMo <= budgetMonth,
			})
		}

		add("OnDemand", "", onDemand)
		add("Reserved", "1yr", res1yr)
		add("Reserved", "3yr", res3yr)

		results[provider] = list
	}

	return results, nil
}

// Get cluster costs for a specific provider only
func CalculateClusterCostsForProvider(
	ctx context.Context,
	pool *pgxpool.Pool,
	provider string,
	best rules.CandidateScore,
	nodeCount int,
	region string,
	budgetMonth float64,
) ([]ClusterCostResult, error) {

	list := []ClusterCostResult{}

	// Calculate node costs for the specific provider and region
	nodeCost, err := CalculateCostsForProviderInRegion(ctx, pool, provider, best, region)
	if err != nil {
		return list, err
	}

	if !nodeCost.FoundMatches {
		return list, nil
	}

	// Get control plane price
	cpTier, cpHour, err := GetControlPlanePrice(ctx, pool, provider)
	if err != nil {
		return list, err
	}
	cpMonth := cpHour * HoursPerMonth()

	var onDemand, res1yr, res3yr *PurchaseOption

	for _, po := range nodeCost.PurchaseOptions {
		t := strings.ToLower(po.Type)
		ll := strings.ToLower(po.LeaseContractLength)

		switch {
		case t == "ondemand" || po.Type == "":
			onDemand = &po
		case t == "reserved" && strings.Contains(ll, "1"):
			res1yr = &po
		case t == "reserved" && strings.Contains(ll, "3"):
			res3yr = &po
		}
	}

	add := func(label, lease string, po *PurchaseOption) {
		if po == nil {
			return
		}

		nHr := po.PricePerHour
		nMo := nHr * HoursPerMonth()

		totalHr := float64(nodeCount)*nHr + cpHour
		totalMo := totalHr * HoursPerMonth()

		list = append(list, ClusterCostResult{
			Provider:          provider,
			PurchaseType:      label,
			LeaseContractType: lease,
			InstanceType:      nodeCost.InstanceType,
			Region:            nodeCost.Region,
			Nodes:             nodeCount,

			PricePerNodeHour:  round(nHr, 5),
			PricePerNodeMonth: round(nMo, 2),

			ControlPlaneTier:  cpTier,
			ControlPlaneHour:  round(cpHour, 5),
			ControlPlaneMonth: round(cpMonth, 2),

			TotalHour:    round(totalHr, 5),
			TotalMonth:   round(totalMo, 2),
			BudgetMonth:  budgetMonth,
			WithinBudget: budgetMonth > 0 && totalMo <= budgetMonth,
		})
	}

	add("OnDemand", "", onDemand)
	add("Reserved", "1yr", res1yr)
	add("Reserved", "3yr", res3yr)

	return list, nil
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

// MATCH functions
func calcForProvider(ctx context.Context, pool *pgxpool.Pool, provider, table string, vcpu int, mem float64) (CostResult, error) {
	cr := CostResult{Provider: provider}

	exactMatches, err := findExactMatches(ctx, pool, provider, table, vcpu, mem)
	if err != nil {
		return cr, fmt.Errorf("finding exact matches failed: %w", err)
	}

	if len(exactMatches) > 0 {
		return processExactMatches(exactMatches, provider, vcpu, mem), nil
	}

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

// Calculate for specific provider and region
func calcForProviderInRegion(ctx context.Context, pool *pgxpool.Pool, provider, table string, vcpu int, mem float64, region string) (CostResult, error) {
	cr := CostResult{Provider: provider}

	exactMatches, err := findExactMatchesInRegion(ctx, pool, provider, table, vcpu, mem, region)
	if err != nil {
		return cr, fmt.Errorf("finding exact matches in region failed: %w", err)
	}

	if len(exactMatches) > 0 {
		result := processExactMatches(exactMatches, provider, vcpu, mem)
		result.Region = region
		return result, nil
	}

	nearestMatches, err := findNearestMatchesInRegion(ctx, pool, provider, table, vcpu, mem, region)
	if err != nil {
		return cr, fmt.Errorf("finding nearest matches in region failed: %w", err)
	}

	if len(nearestMatches) > 0 {
		result := processNearestMatches(nearestMatches, provider, vcpu, mem)
		result.Region = region
		return result, nil
	}

	cr.FoundMatches = false
	cr.MatchCount = 0
	cr.Note = fmt.Sprintf("no matching price rows found in region: %s", region)
	return cr, nil
}

func findExactMatches(ctx context.Context, pool *pgxpool.Pool, provider, table string, vcpu int, mem float64) ([]priceRow, error) {
	query := ""

	switch provider {
	case "aws":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, lease_contract_length, instance_family, NULL
			FROM aws_compute_prices
			WHERE vcpu = $1 AND memory_gb = $2 
			  AND price_per_hour IS NOT NULL 
			  AND price_per_hour <= 10
			ORDER BY price_per_hour ASC
		`
	case "azure":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, lease_contract_length, service_family, NULL
			FROM azure_compute_prices
			WHERE vcpu = $1 AND memory_gb = $2 
			  AND price_per_hour IS NOT NULL
			ORDER BY price_per_hour ASC
		`
	case "gcp":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, NULL, resource_family, usage_type
			FROM gcp_compute_prices
			WHERE vcpu = $1 AND memory_gb = $2 
			  AND price_per_hour IS NOT NULL 
			  AND price_per_hour <= 10
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
		if err := rows.Scan(
			&r.SKUID, &r.InstanceType, &r.Region, &r.VCPU, &r.MemoryGB,
			&r.PriceHour, &r.Currency, &r.Unit, &r.FetchedAt,
			&r.PurchaseOption, &r.LeaseContractLength, &r.ServiceFamily, &r.UsageType,
		); err != nil {
			return nil, err
		}
		r.Distance = 0
		matches = append(matches, r)
	}

	return matches, nil
}

// Find exact matches in specific region
func findExactMatchesInRegion(ctx context.Context, pool *pgxpool.Pool, provider, table string, vcpu int, mem float64, region string) ([]priceRow, error) {
	query := ""

	switch provider {
	case "aws":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, lease_contract_length, instance_family, NULL
			FROM aws_compute_prices
			WHERE vcpu = $1 AND memory_gb = $2 
			  AND region = $3 
			  AND price_per_hour IS NOT NULL 
			  AND price_per_hour <= 10
			ORDER BY price_per_hour ASC
		`
	case "azure":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, lease_contract_length, service_family, NULL
			FROM azure_compute_prices
			WHERE vcpu = $1 AND memory_gb = $2 
			  AND region = $3 
			  AND price_per_hour IS NOT NULL
			ORDER BY price_per_hour ASC
		`
	case "gcp":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, NULL, resource_family, usage_type
			FROM gcp_compute_prices
			WHERE vcpu = $1 AND memory_gb = $2 
			  AND region = $3 
			  AND price_per_hour IS NOT NULL 
			  AND price_per_hour <= 10
			ORDER BY price_per_hour ASC
		`
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}

	rows, err := pool.Query(ctx, query, vcpu, mem, region)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var matches []priceRow
	for rows.Next() {
		var r priceRow
		if err := rows.Scan(
			&r.SKUID, &r.InstanceType, &r.Region, &r.VCPU, &r.MemoryGB,
			&r.PriceHour, &r.Currency, &r.Unit, &r.FetchedAt,
			&r.PurchaseOption, &r.LeaseContractLength, &r.ServiceFamily, &r.UsageType,
		); err != nil {
			return nil, err
		}
		r.Distance = 0
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
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, lease_contract_length, instance_family, NULL,
			       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
			FROM aws_compute_prices
			WHERE price_per_hour IS NOT NULL 
			  AND price_per_hour <= 10
			ORDER BY dist ASC, price_per_hour ASC
			LIMIT 20
		`
	case "azure":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, lease_contract_length, service_family, NULL,
			       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
			FROM azure_compute_prices
			WHERE price_per_hour IS NOT NULL
			ORDER BY dist ASC, price_per_hour ASC
			LIMIT 20
		`
	case "gcp":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, NULL, resource_family, usage_type,
			       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
			FROM gcp_compute_prices
			WHERE price_per_hour IS NOT NULL 
			  AND price_per_hour <= 10
			ORDER BY dist ASC, price_per_hour ASC
			LIMIT 20
		`
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}

	rows, err := pool.Query(ctx, query, vcpu, mem)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []priceRow
	for rows.Next() {
		var r priceRow
		if err := rows.Scan(
			&r.SKUID, &r.InstanceType, &r.Region, &r.VCPU, &r.MemoryGB,
			&r.PriceHour, &r.Currency, &r.Unit, &r.FetchedAt,
			&r.PurchaseOption, &r.LeaseContractLength, &r.ServiceFamily, &r.UsageType,
			&r.Distance,
		); err != nil {
			return nil, err
		}
		matches = append(matches, r)
	}

	return matches, nil
}

// Find nearest matches in specific region
func findNearestMatchesInRegion(ctx context.Context, pool *pgxpool.Pool, provider, table string, vcpu int, mem float64, region string) ([]priceRow, error) {
	query := ""

	switch provider {
	case "aws":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, lease_contract_length, instance_family, NULL,
			       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
			FROM aws_compute_prices
			WHERE region = $3 
			  AND price_per_hour IS NOT NULL 
			  AND price_per_hour <= 10
			ORDER BY dist ASC, price_per_hour ASC
			LIMIT 20
		`
	case "azure":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, lease_contract_length, service_family, NULL,
			       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
			FROM azure_compute_prices
			WHERE region = $3 
			  AND price_per_hour IS NOT NULL
			ORDER BY dist ASC, price_per_hour ASC
			LIMIT 20
		`
	case "gcp":
		query = `
			SELECT sku_id, instance_type, region, vcpu, memory_gb, price_per_hour, currency, unit,
			       to_char(fetched_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       purchase_option, NULL, resource_family, usage_type,
			       (abs(vcpu - $1) + abs(memory_gb - $2)) as dist
			FROM gcp_compute_prices
			WHERE region = $3 
			  AND price_per_hour IS NOT NULL 
			  AND price_per_hour <= 10
			ORDER BY dist ASC, price_per_hour ASC
			LIMIT 20
		`
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}

	rows, err := pool.Query(ctx, query, vcpu, mem, region)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []priceRow
	for rows.Next() {
		var r priceRow
		if err := rows.Scan(
			&r.SKUID, &r.InstanceType, &r.Region, &r.VCPU, &r.MemoryGB,
			&r.PriceHour, &r.Currency, &r.Unit, &r.FetchedAt,
			&r.PurchaseOption, &r.LeaseContractLength, &r.ServiceFamily, &r.UsageType,
			&r.Distance,
		); err != nil {
			return nil, err
		}
		matches = append(matches, r)
	}

	return matches, nil
}

func processExactMatches(matches []priceRow, provider string, targetVCPU int, targetMem float64) CostResult {
	var affordableMatches []priceRow
	for _, match := range matches {
		if provider == "azure" && match.PurchaseOption != nil {
			po := strings.ToLower(*match.PurchaseOption)
			if strings.Contains(po, "reserved") || strings.Contains(po, "savings") {
				affordableMatches = append(affordableMatches, match)
				continue
			}
		}

		if match.PriceHour != nil && *match.PriceHour <= 10 {
			affordableMatches = append(affordableMatches, match)
		}
	}

	if len(affordableMatches) == 0 {
		return CostResult{
			Provider:     provider,
			FoundMatches: false,
			MatchCount:   0,
			Note:         "no affordable exact matches found",
		}
	}

	matches = affordableMatches

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
	var affordableMatches []priceRow
	for _, match := range matches {
		if provider == "azure" && match.PurchaseOption != nil {
			po := strings.ToLower(*match.PurchaseOption)
			if strings.Contains(po, "reserved") || strings.Contains(po, "savings") {
				affordableMatches = append(affordableMatches, match)
				continue
			}
		}

		if match.PriceHour != nil && *match.PriceHour <= 10 {
			affordableMatches = append(affordableMatches, match)
		}
	}

	if len(affordableMatches) == 0 {
		return CostResult{
			Provider:     provider,
			FoundMatches: false,
			MatchCount:   0,
			Note:         "no affordable options found",
		}
	}

	matches = affordableMatches

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
						po == ""
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

func groupPurchaseOptions(rows []priceRow, provider string) []PurchaseOption {
	purchaseOptions := make([]PurchaseOption, 0)
	optionMap := make(map[string]PurchaseOption)

	var onDemandPrice float64

	var filteredRows []priceRow
	for _, row := range rows {
		if row.PriceHour == nil || *row.PriceHour <= 0 {
			continue
		}
		if row.Unit != nil && strings.ToLower(*row.Unit) == "quantity" {
			continue
		}
		if provider != "azure" && *row.PriceHour > 10 {
			continue
		}
		filteredRows = append(filteredRows, row)
	}

	for _, row := range filteredRows {
		optionType := "Other"
		leaseLength := ""
		pricePerHour := *row.PriceHour
		note := ""

		if row.PurchaseOption != nil {
			po := strings.ToLower(*row.PurchaseOption)

			switch {
			case strings.Contains(po, "ondemand") || strings.Contains(po, "payg") ||
				strings.Contains(po, "consumption") || po == "on_demand" ||
				strings.Contains(po, "on demand") || po == "":
				optionType = "OnDemand"
				onDemandPrice = pricePerHour

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

				if provider == "azure" && optionType == "Reserved" {
					var totalMonthsInLease float64
					if strings.Contains(strings.ToLower(leaseLength), "1") || leaseLength == "1yr" {
						totalMonthsInLease = 12
					} else if strings.Contains(strings.ToLower(leaseLength), "3") || leaseLength == "3yr" {
						totalMonthsInLease = 36
					} else {
						totalMonthsInLease = 12
					}

					if pricePerHour > 100 {
						monthlyCost := pricePerHour / totalMonthsInLease
						pricePerHour = monthlyCost / 720
						note = fmt.Sprintf("Converted from total reservation cost: $%.2f/%d months/720 hrs",
							*row.PriceHour, int(totalMonthsInLease))
					}
				}
			}
		} else {
			// If no purchase option specified, assume On-Demand
			optionType = "OnDemand"
			onDemandPrice = pricePerHour
		}

		key := fmt.Sprintf("%s_%s", optionType, leaseLength)

		currency := "USD"
		if row.Currency != nil {
			currency = *row.Currency
		}

		unit := "Hrs"

		monthlyCost := pricePerHour * HoursPerMonth()

		// Round the values
		pricePerHour = math.Round(pricePerHour*100000) / 100000
		monthlyCost = math.Round(monthlyCost*100) / 100

		po := PurchaseOption{
			Type:                optionType,
			LeaseContractLength: leaseLength,
			PricePerHour:        pricePerHour,
			MonthlyCost:         monthlyCost,
			Currency:            currency,
			Unit:                unit,
		}

		if note != "" {
			po.Note = note
		}

		optionMap[key] = po
	}

	// Calculate savings percentages for Reserved instances
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
