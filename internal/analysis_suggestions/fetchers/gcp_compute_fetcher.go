package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"golang.org/x/time/rate"
	cloudbilling "google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/option"
)

type GcpComputePrice struct {
	ID           string                 `json:"id"`
	Provider     string                 `json:"provider"`
	SKUID        string                 `json:"sku_id"`
	Region       string                 `json:"region"`
	Description  string                 `json:"description"`
	Unit         string                 `json:"unit,omitempty"`
	PricePerUnit *float64               `json:"price_per_hour"`
	Currency     string                 `json:"currency,omitempty"`
	VCPU         *int                   `json:"vcpu,omitempty"`
	MemoryGB     *float64               `json:"memory_gb,omitempty"`
	FetchedAt    time.Time              `json:"fetched_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

var (
	reVCPUDesc      = regexp.MustCompile(`(?i)(\b[0-9]{1,4})\s*(v?cpu|vcpu|v-cpu|cores?)\b`)
	reMemDesc       = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*(GiB|GB|gib|gb)\b`)
	reMachineSimple = regexp.MustCompile(`([a-z0-9]+-[a-z0-9]+-[0-9]+)`)
)

func main() {
	outDir := "out/asm"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir out: %v", err)
	}
	outFile := filepath.Join(outDir, "gcp_compute_prices.csv")

	limiter := rate.NewLimiter(rate.Limit(4), 8)
	ctx := context.Background()

	creds, _ := google.FindDefaultCredentials(ctx, cloudbilling.CloudPlatformScope)
	var opts []option.ClientOption
	if creds != nil && creds.JSON != nil {
		opts = append(opts, option.WithCredentialsJSON(creds.JSON))
	}

	billingSvc, err := cloudbilling.NewService(ctx, opts...)
	if err != nil {
		log.Fatalf("cloudbilling.NewService: %v", err)
	}

	log.Printf("Listing services from Cloud Billing Catalog API...")
	services, err := listServices(ctx, billingSvc)
	if err != nil {
		log.Fatalf("listServices: %v", err)
	}

	computeServices := pickComputeServices(services)
	if len(computeServices) == 0 {
		log.Fatalf("no compute services found in billing catalog")
	}
	log.Printf("Found %d compute services to query", len(computeServices))

	f, err := os.Create(outFile)
	if err != nil {
		log.Fatalf("create out file: %v", err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	header := []string{
		"id", "provider", "sku_id", "region", "description", "unit",
		"price_per_hour", "currency", "vcpu", "memory_gb", "fetched_at",
	}
	if err := writer.Write(header); err != nil {
		log.Fatalf("write header failed: %v", err)
	}

	total := 0
	for _, svc := range computeServices {
		svcName := svc.Name
		log.Printf("Fetching SKUs for service: %s (%s)", svcName, svc.DisplayName)

		pageToken := ""
		for {
			if err := limiter.Wait(ctx); err != nil {
				log.Fatalf("rate limiter: %v", err)
			}
			call := billingSvc.Services.Skus.List(svcName).PageSize(500)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			resp, err := call.Do()
			if err != nil {
				log.Fatalf("Services.Skus.List(%s) failed: %v", svcName, err)
			}

			for _, sku := range resp.Skus {
				if sku.Category != nil && !strings.EqualFold(sku.Category.ResourceFamily, "Compute") {
					continue
				}

				regions := sku.ServiceRegions
				if len(regions) == 0 {
					regions = []string{"global"}
				}

				for _, region := range regions {
					price, unit, currency := extractPriceFromSKU(sku)
					if price == nil {
						continue
					}

					vcpu, mem := parseVcpuMemFromSKU(sku)
					out := GcpComputePrice{
						ID:           fmt.Sprintf("gcp|%s|%s", sanitizeName(sku.Name), region),
						Provider:     "gcp",
						SKUID:        sku.Name,
						Region:       region,
						Description:  sku.Description,
						Unit:         unit,
						PricePerUnit: price,
						Currency:     currency,
						VCPU:         vcpu,
						MemoryGB:     mem,
						FetchedAt:    time.Now().UTC(),
					}
					total++

					record := []string{
						out.ID,
						out.Provider,
						out.SKUID,
						out.Region,
						strings.ReplaceAll(out.Description, "\n", " "),
						out.Unit,
						fmt.Sprintf("%f", *out.PricePerUnit),
						out.Currency,
						intPtrToStr(out.VCPU),
						floatPtrToStr(out.MemoryGB),
						out.FetchedAt.Format(time.RFC3339),
					}
					if err := writer.Write(record); err != nil {
						log.Fatalf("write record failed: %v", err)
					}

					if total%200 == 0 {
						writer.Flush()
					}
				}
			}
			if resp.NextPageToken == "" {
				break
			}
			pageToken = resp.NextPageToken
			time.Sleep(time.Duration(rand.Intn(200)+100) * time.Millisecond)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatalf("flush failed: %v", err)
	}
	log.Printf("✅ Done. Wrote %d records to %s", total, outFile)
}

func parseVcpuMemFromSKU(sku *cloudbilling.Sku) (*int, *float64) {
	if sku.Description != "" {
		desc := sku.Description

		var vcpu *int
		if m := reVCPUDesc.FindStringSubmatch(desc); len(m) >= 2 {
			if n, err := strconvAtoiSafe(m[1]); err == nil {
				vcpu = &n
			}
		}

		mem := parseMemFromText(desc)

		if vcpu != nil || mem != nil {
			return vcpu, mem
		}
	}

	src := sku.Name + " " + sku.Description
	if m := reMachineSimple.FindStringSubmatch(strings.ToLower(src)); len(m) >= 2 {
		token := m[1]
		parts := strings.Split(token, "-")
		if len(parts) >= 3 {
			last := parts[len(parts)-1]
			if n, err := strconvAtoiSafe(last); err == nil {
				vcpu := n
				return &vcpu, nil
			}
		}
	}

	return nil, nil
}
func parseMemFromText(s string) *float64 {
	if m := reMemDesc.FindStringSubmatch(s); len(m) >= 2 {
		if f, err := strconvParseFloatSafe(m[1]); err == nil {
			return &f
		}
	}
	return nil
}

func strconvAtoiSafe(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func strconvParseFloatSafe(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func listServices(ctx context.Context, svc *cloudbilling.APIService) ([]*cloudbilling.Service, error) {
	var out []*cloudbilling.Service
	pageToken := ""
	for {
		call := svc.Services.List().PageSize(200)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Services...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return out, nil
}

func pickComputeServices(services []*cloudbilling.Service) []*cloudbilling.Service {
	var out []*cloudbilling.Service
	for _, s := range services {
		if s == nil {
			continue
		}
		name := strings.ToLower(s.DisplayName)
		if strings.Contains(name, "compute") || strings.Contains(name, "compute engine") {
			out = append(out, s)
		}
	}
	return out
}

func extractPriceFromSKU(sku *cloudbilling.Sku) (*float64, string, string) {
	for _, p := range sku.PricingInfo {
		if p == nil || p.PricingExpression == nil {
			continue
		}
		pe := p.PricingExpression
		if len(pe.TieredRates) == 0 {
			continue
		}
		tr := pe.TieredRates[0]
		if tr.UnitPrice == nil {
			continue
		}
		units := float64(tr.UnitPrice.Units)
		nanos := float64(tr.UnitPrice.Nanos) / 1e9
		price := units + nanos
		currency := "USD"
		unit := strings.TrimSpace(pe.UsageUnit)
		if math.IsNaN(price) || price == 0 {
			return nil, unit, currency
		}
		return &price, unit, currency
	}
	return nil, "", ""
}

func sanitizeName(s string) string {
	return strings.ReplaceAll(s, "/", "|")
}

func intPtrToStr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

func floatPtrToStr(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *p)
}
