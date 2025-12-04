package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"math/rand"
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
	ID             string    `json:"id"`
	Provider       string    `json:"provider"`
	SKUID          string    `json:"sku_id"`
	Region         string    `json:"region"`
	InstanceType   string    `json:"instance_type"`
	ResourceFamily string    `json:"resource_family"`
	VCPU           *int      `json:"vcpu,omitempty"`
	MemoryGB       *float64  `json:"memory_gb,omitempty"`
	PricePerHour   *float64  `json:"price_per_hour"`
	Currency       string    `json:"currency,omitempty"`
	Unit           string    `json:"unit,omitempty"`
	PurchaseOption string    `json:"purchase_option"`
	UsageType      string    `json:"usage_type"`
	FetchedAt      time.Time `json:"fetched_at"`
}

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
		"id", "provider", "sku_id", "region", "instance_type", "resource_family",
		"vcpu", "memory_gb", "price_per_hour", "currency", "unit",
		"purchase_option", "usage_type", "fetched_at",
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
					instanceType := extractInstanceType(sku)
					resourceFamily := ""
					usageType := ""
					purchaseOption := "OnDemand"

					if sku.Category != nil {
						resourceFamily = sku.Category.ResourceFamily
						usageType = sku.Category.UsageType
					}

					// Determine purchase option
					if strings.Contains(strings.ToLower(sku.Description), "preemptible") {
						purchaseOption = "Preemptible"
					} else if strings.Contains(strings.ToLower(sku.Description), "commitment") ||
						strings.Contains(strings.ToLower(sku.Description), "reserved") {
						purchaseOption = "Reserved"
					}

					out := GcpComputePrice{
						ID:             fmt.Sprintf("gcp|%s|%s", sanitizeName(sku.Name), region),
						Provider:       "gcp",
						SKUID:          sku.Name,
						Region:         region,
						InstanceType:   instanceType,
						ResourceFamily: resourceFamily,
						VCPU:           vcpu,
						MemoryGB:       mem,
						PricePerHour:   price,
						Currency:       currency,
						Unit:           unit,
						PurchaseOption: purchaseOption,
						UsageType:      usageType,
						FetchedAt:      time.Now().UTC(),
					}
					total++

					record := []string{
						out.ID,
						out.Provider,
						out.SKUID,
						out.Region,
						out.InstanceType,
						out.ResourceFamily,
						intPtrToStr(out.VCPU),
						floatPtrToStr(out.MemoryGB),
						fmt.Sprintf("%f", *out.PricePerHour),
						out.Currency,
						out.Unit,
						out.PurchaseOption,
						out.UsageType,
						out.FetchedAt.Format(time.RFC3339),
					}
					if err := writer.Write(record); err != nil {
						log.Fatalf("write record failed: %v", err)
					}

					if total%200 == 0 {
						writer.Flush()
						log.Printf("Processed %d records...", total)
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

func extractInstanceType(sku *cloudbilling.Sku) string {
	// Try to extract instance type from description
	desc := strings.ToLower(sku.Description)

	// Look for common GCP instance type patterns
	patterns := []string{
		"n1-standard", "n1-highmem", "n1-highcpu",
		"n2-standard", "n2-highmem", "n2-highcpu", "n2d-standard", "n2d-highmem", "n2d-highcpu",
		"e2-standard", "e2-highmem", "e2-highcpu", "e2-small", "e2-micro",
		"c2-standard", "c2d-standard",
		"m1-", "m2-", "m3-",
		"a2-",
	}

	for _, pattern := range patterns {
		if strings.Contains(desc, pattern) {
			// Extract the full instance type (e.g., "n1-standard-4")
			start := strings.Index(desc, pattern)
			if start != -1 {
				remaining := desc[start:]
				// Find the end of the instance type (space, comma, or end of string)
				end := len(remaining)
				for i, char := range remaining {
					if char == ' ' || char == ',' || char == '.' {
						end = i
						break
					}
				}
				return remaining[:end]
			}
		}
	}

	// Fallback: extract from SKU name
	if strings.Contains(sku.Name, "services/6F81-5844-456A") { // Compute Engine service
		parts := strings.Split(sku.Name, "/")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			if len(lastPart) > 0 {
				return lastPart
			}
		}
	}

	return ""
}

func parseVcpuMemFromSKU(sku *cloudbilling.Sku) (*int, *float64) {
	if sku.Description != "" {
		desc := sku.Description

		var vcpu *int
		if m := reVCPUDesc.FindStringSubmatch(desc); len(m) >= 2 {
			if n, err := strconv.Atoi(strings.TrimSpace(m[1])); err == nil {
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
			if n, err := strconv.Atoi(strings.TrimSpace(last)); err == nil {
				vcpu := n
				return &vcpu, nil
			}
		}
	}

	return nil, nil
}

func parseMemFromText(s string) *float64 {
	if m := reMemDesc.FindStringSubmatch(s); len(m) >= 2 {
		if f, err := strconv.ParseFloat(strings.TrimSpace(m[1]), 64); err == nil {
			return &f
		}
	}
	return nil
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
		if tr.UnitPrice.CurrencyCode != "" {
			currency = tr.UnitPrice.CurrencyCode
		}
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
