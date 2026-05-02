package costcal

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/rules"
)

const recommendMaxConcurrency = 14

// cheapest cluster plan across all regions/providers
type GlobalRecommendation struct {
	FitsBudget          bool                `json:"fits_budget"`
	Recommended         *ClusterCostResult  `json:"recommended,omitempty"`
	RunnersUp           []ClusterCostResult `json:"runners_up"`
	Rationale           []string            `json:"rationale"`
	RegionJobsEvaluated int                 `json:"region_jobs_evaluated"`
	PlansEvaluated      int                 `json:"plans_evaluated"`
	WithinBudgetPlans   int                 `json:"within_budget_plans"`
}

func clusterPlanKey(c ClusterCostResult) string {
	return c.Provider + "|" + c.Region + "|" + c.PurchaseType + "|" + c.LeaseContractType + "|" + c.InstanceType
}

func sortClusterPlansByCost(plans []ClusterCostResult) {
	sort.Slice(plans, func(i, j int) bool {
		a, b := plans[i], plans[j]
		if a.TotalMonth != b.TotalMonth {
			return a.TotalMonth < b.TotalMonth
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		if a.Region != b.Region {
			return a.Region < b.Region
		}
		if a.PurchaseType != b.PurchaseType {
			return a.PurchaseType < b.PurchaseType
		}
		return a.LeaseContractType < b.LeaseContractType
	})
}

func BuildGlobalRecommendation(
	ctx context.Context,
	db *sql.DB,
	best rules.CandidateScore,
	nodeCount int,
	budgetMonth float64,
) (*GlobalRecommendation, error) {
	vcpu := best.Candidate.Spec.VCPU
	mem := best.Candidate.Spec.MemoryGB

	type job struct {
		provider, region string
	}
	var jobs []job
	for _, p := range []string{"aws", "azure", "gcp"} {
		regions, err := GetRegionsForCandidateSpec(ctx, db, p, vcpu, mem)
		if err != nil {
			return nil, fmt.Errorf("regions for %s: %w", p, err)
		}
		for _, r := range regions {
			jobs = append(jobs, job{provider: p, region: r})
		}
	}

	out := make([]ClusterCostResult, 0, len(jobs)*3)
	var mu sync.Mutex
	sem := make(chan struct{}, recommendMaxConcurrency)
	var wg sync.WaitGroup

	for _, j := range jobs {
		j := j
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}
			rows, err := CalculateClusterCostsForProvider(ctx, db, j.provider, best, nodeCount, j.region, budgetMonth)
			if err != nil || len(rows) == 0 {
				return
			}
			mu.Lock()
			out = append(out, rows...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(out) == 0 {
		return &GlobalRecommendation{
			Rationale: []string{
				"No compute prices were found for this workload across AWS and Azure regions in the catalog.",
			},
			RegionJobsEvaluated: len(jobs),
		}, nil
	}

	sortClusterPlansByCost(out)

	withinBudget := make([]ClusterCostResult, 0)
	for i := range out {
		if out[i].WithinBudget {
			withinBudget = append(withinBudget, out[i])
		}
	}
	sortClusterPlansByCost(withinBudget)

	var rec ClusterCostResult
	fits := false

	switch {
	case budgetMonth > 0 && len(withinBudget) > 0:
		rec = withinBudget[0]
		fits = true
	case budgetMonth > 0 && len(withinBudget) == 0:
		rec = out[0]
		fits = false
	default:
		rec = out[0]
		fits = true
	}

	recPtr := rec
	runners := make([]ClusterCostResult, 0, 10)
	winKey := clusterPlanKey(rec)
	for _, c := range out {
		if clusterPlanKey(c) == winKey {
			continue
		}
		runners = append(runners, c)
		if len(runners) >= 10 {
			break
		}
	}

	var secondWithin *ClusterCostResult
	if fits && len(withinBudget) > 1 {
		s := withinBudget[1]
		secondWithin = &s
	}

	rationale := buildRecommendationRationale(
		budgetMonth, len(withinBudget), fits, rec, secondWithin,
	)

	return &GlobalRecommendation{
		FitsBudget:          fits,
		Recommended:         &recPtr,
		RunnersUp:           runners,
		Rationale:           rationale,
		RegionJobsEvaluated: len(jobs),
		PlansEvaluated:      len(out),
		WithinBudgetPlans:   len(withinBudget),
	}, nil
}

func buildRecommendationRationale(
	budgetMonth float64,
	withinCount int,
	fits bool,
	winner ClusterCostResult,
	secondWithinBudget *ClusterCostResult,
) []string {
	lines := make([]string, 0, 6)

	if budgetMonth > 0 {
		lines = append(lines, fmt.Sprintf("Monthly budget from the design: $%.2f.", budgetMonth))
		if fits {
			lines = append(lines, fmt.Sprintf(
				"Cheapest option under budget: $%.2f/month — %s %s in %s (%s%s).",
				winner.TotalMonth,
				winner.Provider,
				winner.InstanceType,
				winner.Region,
				winner.PurchaseType,
				leaseLabel(winner.LeaseContractType),
			))
			if withinCount > 1 {
				lines = append(lines, fmt.Sprintf("This is the lowest monthly total among them."))
			}
		} else {
			over := winner.TotalMonth - budgetMonth
			lines = append(lines, "No plan in the catalog is within the monthly budget at the evaluated regions.")
			lines = append(lines, fmt.Sprintf(
				"Absolute lowest monthly total is $%.2f (%s %s, %s, %s%s), which is $%.2f over budget.",
				winner.TotalMonth,
				winner.Provider,
				winner.InstanceType,
				winner.Region,
				winner.PurchaseType,
				leaseLabel(winner.LeaseContractType),
				over,
			))
		}
	} else {
		lines = append(lines, "No monthly budget was set in the design; recommending the lowest monthly total across all regions.")
		lines = append(lines, fmt.Sprintf(
			"Lowest monthly total: $%.2f — %s %s in %s (%s%s).",
			winner.TotalMonth,
			winner.Provider,
			winner.InstanceType,
			winner.Region,
			winner.PurchaseType,
			leaseLabel(winner.LeaseContractType),
		))
	}

	if secondWithinBudget != nil && budgetMonth > 0 {
		delta := secondWithinBudget.TotalMonth - winner.TotalMonth
		if delta > 0.005 {
			pct := 100.0 * delta / winner.TotalMonth
			lines = append(lines, fmt.Sprintf(
				"Next cheapest within budget is $%.2f/month (+$%.2f vs this pick, +%.1f%%) on %s in %s.",
				secondWithinBudget.TotalMonth,
				delta,
				pct,
				secondWithinBudget.Provider,
				secondWithinBudget.Region,
			))
		}
	}

	return lines
}

func leaseLabel(lease string) string {
	if lease == "" {
		return ""
	}
	return " " + lease
}
