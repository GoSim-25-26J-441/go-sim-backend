package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	pricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	"golang.org/x/time/rate"
)

type CloudComputePrice struct {
	ID                  string    `json:"id"`
	Provider            string    `json:"provider"`
	SKUID               string    `json:"sku_id"`
	Region              string    `json:"region"`
	InstanceType        string    `json:"instance_type"`
	InstanceFamily      string    `json:"instanceFamily"`
	VCPU                *int      `json:"vcpu,omitempty"`
	MemoryGB            *float64  `json:"memory_gb,omitempty"`
	PricePerHour        *float64  `json:"price_per_hour,omitempty"`
	Currency            string    `json:"currency,omitempty"`
	Unit                string    `json:"unit,omitempty"`
	PurchaseOption      string    `json:"purchase_option,omitempty"`
	LeaseContractLength string    `json:"lease_contract_length,omitempty"`
	FetchedAt           time.Time `json:"fetched_at"`
}

type FetchConfig struct {
	MaxRecords     int
	RateLimit      rate.Limit
	BurstSize      int
	BufferSize     int
	Workers        int
	BackoffInitial time.Duration
	BackoffMax     time.Duration
	MaxRetries     int
}

var (
	reAwsMemoryGB = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*(GiB|GB|gib|gb)`)
	reInstToken   = regexp.MustCompile(`([a-z]+[0-9]+[a-z0-9\.-]*)`)
)

func main() {
	outDir := "out/asm"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir out: %v", err)
	}
	csvPath := filepath.Join(outDir, "aws_compute_prices.csv")
	txtPath := filepath.Join(outDir, "aws_compute_prices.txt")

	config := FetchConfig{
		MaxRecords:     800000,
		RateLimit:      8,
		BurstSize:      16,
		BufferSize:     500,
		Workers:        4,
		BackoffInitial: 1 * time.Second,
		BackoffMax:     30 * time.Second,
		MaxRetries:     3,
	}

	ctx := context.Background()
	cfg, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion("us-east-1"))
	if err != nil {
		log.Fatalf("aws config load: %v", err)
	}
	client := pricing.NewFromConfig(cfg)

	log.Printf("Starting AWS EC2 pricing fetcher with config: MaxRecords=%d, RateLimit=%v, Workers=%d",
		config.MaxRecords, config.RateLimit, config.Workers)
	if err := fetchAWSComputeOptimized(ctx, client, config, csvPath, txtPath); err != nil {
		log.Fatalf("fetch failed: %v", err)
	}
	log.Println("Done.")
}

func fetchAWSComputeOptimized(ctx context.Context, client *pricing.Client, cfg FetchConfig, csvPath, txtPath string) error {
	csvF, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("create csv: %w", err)
	}
	defer csvF.Close()
	csvW := bufio.NewWriter(csvF)

	txtF, err := os.Create(txtPath)
	if err != nil {
		return fmt.Errorf("create txt: %w", err)
	}
	defer txtF.Close()
	tw := tabwriter.NewWriter(txtF, 0, 4, 2, ' ', 0)

	header := []string{
		"id", "provider", "sku_id", "region", "instance_type", "instanceFamily",
		"vcpu", "memory_gb", "price_per_hour", "currency", "unit",
		"purchase_option", "lease_contract_length", "fetched_at",
	}
	if _, err := csvW.WriteString(strings.Join(header, ",") + "\n"); err != nil {
		return fmt.Errorf("csv header: %w", err)
	}
	fmt.Fprintf(tw, "%s\n", strings.Join(header, "\t"))
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("txt header flush: %w", err)
	}

	limiter := rate.NewLimiter(cfg.RateLimit, cfg.BurstSize)

	recordChan := make(chan *CloudComputePrice, cfg.BufferSize)
	errChan := make(chan error, cfg.Workers)
	var wg sync.WaitGroup
	var writerMutex sync.Mutex

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rec := range recordChan {
				if rec == nil {
					return
				}
				writerMutex.Lock()

				csvQuote := func(s string) string {
					if strings.ContainsAny(s, ",\"\n\r") {
						return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
					}
					return s
				}

				id := csvQuote(rec.ID)
				provider := csvQuote(rec.Provider)
				skuID := csvQuote(rec.SKUID)
				region := csvQuote(rec.Region)
				instanceType := csvQuote(rec.InstanceType)
				instanceFamily := csvQuote(rec.InstanceFamily)
				vcpu := csvQuote(nilToStrInt(rec.VCPU))
				memoryGB := csvQuote(nilToStrFloat(rec.MemoryGB))
				price := csvQuote(nilToStrFloat6(rec.PricePerHour))
				currency := csvQuote(rec.Currency)
				unit := csvQuote(rec.Unit)
				purchaseOption := csvQuote(rec.PurchaseOption)
				leaseContractLength := csvQuote(rec.LeaseContractLength)
				fetchedAt := csvQuote(rec.FetchedAt.Format(time.RFC3339Nano))

				csvLine := []string{
					id, provider, skuID, region, instanceType, instanceFamily,
					vcpu, memoryGB, price, currency, unit,
					purchaseOption, leaseContractLength, fetchedAt,
				}

				if _, err := csvW.WriteString(strings.Join(csvLine, ",") + "\n"); err != nil {
					writerMutex.Unlock()
					errChan <- err
					return
				}

				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					rec.ID, rec.Provider, rec.SKUID, rec.Region, rec.InstanceType, rec.InstanceFamily,
					nilToStrInt(rec.VCPU), nilToStrFloat(rec.MemoryGB), nilToStrFloat6(rec.PricePerHour),
					rec.Currency, rec.Unit, rec.PurchaseOption, rec.LeaseContractLength,
					rec.FetchedAt.Format(time.RFC3339Nano))

				writerMutex.Unlock()
			}
		}()
	}

	input := &pricing.GetProductsInput{
		ServiceCode:   aws.String("AmazonEC2"),
		FormatVersion: aws.String("aws_v1"),
	}

	total := 0
	var nextToken *string
	backoff := cfg.BackoffInitial

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errChan:
			if err != nil {
				return fmt.Errorf("writer error: %w", err)
			}
		default:
		}

		if cfg.MaxRecords > 0 && total >= cfg.MaxRecords {
			log.Printf("Reached max records limit: %d", total)
			break
		}

		if err := limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter: %w", err)
		}

		input.NextToken = nextToken

		var resp *pricing.GetProductsOutput
		var err error

		for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
			resp, err = client.GetProducts(ctx, input)
			if err == nil {
				backoff = cfg.BackoffInitial
				break
			}

			if attempt < cfg.MaxRetries {
				log.Printf("Attempt %d failed: %v. Retrying in %v...", attempt+1, err, backoff)
				select {
				case <-time.After(backoff):
					backoff = time.Duration(float64(backoff) * 1.5)
					if backoff > cfg.BackoffMax {
						backoff = cfg.BackoffMax
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			} else {
				return fmt.Errorf("GetProducts failed after %d retries: %w", cfg.MaxRetries+1, err)
			}
		}

		for _, pl := range resp.PriceList {
			if cfg.MaxRecords > 0 && total >= cfg.MaxRecords {
				break
			}

			var js map[string]interface{}
			if err := json.Unmarshal([]byte(pl), &js); err != nil {
				continue
			}

			prod, _ := js["product"].(map[string]interface{})
			terms, _ := js["terms"].(map[string]interface{})

			productFamily, _ := prod["productFamily"].(string)
			attributes, _ := prod["attributes"].(map[string]interface{})

			if productFamily == "" && attributes == nil {
				continue
			}

			var instanceType, instanceFamily string
			if attributes != nil {
				if v, ok := attributes["instanceType"].(string); ok {
					instanceType = v
				}
				if v, ok := attributes["instanceFamily"].(string); ok {
					instanceFamily = v
				}
				if instanceFamily == "" && instanceType != "" {
					instanceFamily = extractInstanceFamily(instanceType)
				}
			}
			if instanceType == "" {
				if v, ok := prod["sku"].(string); ok {
					instanceType = extractInstanceToken(v)
				}
			}

			if !isLikelyCompute(productFamily, attributes, instanceType) {
				continue
			}

			region := ""
			if attributes != nil {
				if loc, ok := attributes["location"].(string); ok {
					region = canonicalizeAwsRegion(loc)
				} else if r, ok := attributes["aws:region"].(string); ok {
					region = r
				}
			}

			skuID, _ := prod["sku"].(string)
			if skuID == "" {
				if ps, ok := prod["productSKU"].(string); ok {
					skuID = ps
				}
			}
			if skuID == "" {
				continue
			}

			vcpu, memoryGB := extractSpecs(attributes, pl)

			priceEntries := extractAllPriceEntries(terms)

			for _, pe := range priceEntries {
				rec := &CloudComputePrice{
					ID:                  fmt.Sprintf("aws|%s|%s|%s", skuID, region, pe.EntryID),
					Provider:            "aws",
					SKUID:               skuID,
					Region:              region,
					InstanceType:        instanceType,
					InstanceFamily:      instanceFamily,
					VCPU:                vcpu,
					MemoryGB:            memoryGB,
					PricePerHour:        pe.Price,
					Currency:            pe.Currency,
					Unit:                pe.Unit,
					PurchaseOption:      pe.PurchaseOption,
					LeaseContractLength: pe.LeaseContractLength,
					FetchedAt:           time.Now().UTC(),
				}

				recordChan <- rec
				total++
				if cfg.MaxRecords > 0 && total >= cfg.MaxRecords {
					break
				}
			}
			if cfg.MaxRecords > 0 && total >= cfg.MaxRecords {
				break
			}
		}

		if resp.NextToken == nil || *resp.NextToken == "" {
			break
		}
		nextToken = resp.NextToken

		time.Sleep(time.Duration(rand.Intn(100)+50) * time.Millisecond)
	}

	close(recordChan)
	wg.Wait()

	if err := csvW.Flush(); err != nil {
		return fmt.Errorf("csv flush: %w", err)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("txt flush: %w", err)
	}

	log.Printf("Successfully wrote %d records", total)
	return nil
}

type PriceEntry struct {
	EntryID             string   `json:"entry_id"`
	PurchaseOption      string   `json:"purchase_option"`
	LeaseContractLength string   `json:"lease_contract_length"`
	OfferingClass       string   `json:"offering_class,omitempty"`
	Price               *float64 `json:"price,omitempty"`
	Currency            string   `json:"currency,omitempty"`
	Unit                string   `json:"unit,omitempty"`
	Description         string   `json:"description,omitempty"`
}

func extractAllPriceEntries(terms map[string]interface{}) []PriceEntry {
	out := []PriceEntry{}
	if terms == nil {
		return out
	}

	for termKind, termVal := range terms {
		ltermKind := strings.ToLower(termKind)
		if termMapRoot, ok := termVal.(map[string]interface{}); ok {
			for termCode, termInner := range termMapRoot {
				if tInnerMap, ok := termInner.(map[string]interface{}); ok {
					pe := PriceEntry{EntryID: termCode}

					if strings.Contains(ltermKind, "ondemand") {
						pe.PurchaseOption = "ondemand"
					} else if strings.Contains(ltermKind, "spot") {
						pe.PurchaseOption = "spot"
					} else if strings.Contains(ltermKind, "reserved") || strings.Contains(ltermKind, "reservedinstances") {
						pe.PurchaseOption = "reserved"
						if ta, ok := tInnerMap["termAttributes"].(map[string]interface{}); ok {
							if lease, ok := ta["LeaseContractLength"].(string); ok {
								leaseNorm := strings.TrimSpace(strings.ToLower(lease))
								leaseNorm = strings.ReplaceAll(leaseNorm, " ", "")
								pe.LeaseContractLength = leaseNorm
							}
						}
					} else {
						if _, ok := tInnerMap["priceDimensions"]; ok {
							pe.PurchaseOption = strings.ToLower(termKind)
						}
					}

					if ta, ok := tInnerMap["termAttributes"].(map[string]interface{}); ok {
						if oc, ok := ta["OfferingClass"].(string); ok {
							pe.OfferingClass = oc
						}
					}

					if pds, ok := tInnerMap["priceDimensions"].(map[string]interface{}); ok {
						for pdCode, pdVal := range pds {
							if pdMap, ok := pdVal.(map[string]interface{}); ok {
								pe.EntryID = pdCode
								if ppu, ok := pdMap["pricePerUnit"].(map[string]interface{}); ok {
									found := false
									for cur, val := range ppu {
										switch v := val.(type) {
										case string:
											if f, err := strconv.ParseFloat(v, 64); err == nil {
												pe.Price = &f
												pe.Currency = cur
												found = true
											}
										case float64:
											pe.Price = &v
											pe.Currency = cur
											found = true
										}
										if found {
											break
										}
									}
								}
								if u, ok := pdMap["unit"].(string); ok {
									pe.Unit = u
								}
								if d, ok := pdMap["description"].(string); ok {
									pe.Description = d
								}
								out = append(out, pe)
								break
							}
						}
					}
				}
			}
		}
	}

	for i := range out {
		if out[i].LeaseContractLength != "" {
			l := strings.ToLower(out[i].LeaseContractLength)
			l = strings.ReplaceAll(l, " ", "")
			l = strings.ReplaceAll(l, "years", "yr")
			l = strings.ReplaceAll(l, "year", "yr")
			out[i].LeaseContractLength = l
		}
	}

	return out
}

func summarizeTerms(terms map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	if terms == nil {
		return out
	}
	if on, ok := terms["OnDemand"]; ok {
		if odMap, ok := on.(map[string]interface{}); ok {
			for termCode, term := range odMap {
				if termMap, ok := term.(map[string]interface{}); ok {
					out["type"] = "OnDemand"
					out["term_code"] = termCode
					if pds, ok := termMap["priceDimensions"].(map[string]interface{}); ok {
						for pdCode, pdVal := range pds {
							if pdMap, ok := pdVal.(map[string]interface{}); ok {
								out["price_dimension_code"] = pdCode
								if ppu, ok := pdMap["pricePerUnit"].(map[string]interface{}); ok {
									if usd, ok := ppu["USD"].(string); ok {
										out["price"] = usd
										out["currency"] = "USD"
									} else if usdF, ok := ppu["USD"].(float64); ok {
										out["price"] = fmt.Sprintf("%f", usdF)
										out["currency"] = "USD"
									}
								}
								if u, ok := pdMap["unit"].(string); ok {
									out["unit"] = u
								}
								if d, ok := pdMap["description"].(string); ok {
									out["description"] = d
								}
								return out
							}
						}
					}
				}
			}
		}
	}

	for k := range terms {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "reserved") || strings.Contains(lk, "spot") {
			out["type"] = k
			break
		}
	}
	return out
}

func extractSpecs(attributes map[string]interface{}, rawJSON string) (*int, *float64) {
	var vcpu *int
	var memoryGB *float64

	if attributes != nil {
		if vv, ok := attributes["vcpu"].(string); ok {
			if n, err := strconvAtoiSafe(vv); err == nil {
				vcpu = &n
			}
		}
		if vv, ok := attributes["vCPU"].(string); ok {
			if n, err := strconvAtoiSafe(vv); err == nil {
				vcpu = &n
			}
		}
		if memRaw, ok := attributes["memory"].(string); ok {
			if f, err := parseMemoryString(memRaw); err == nil {
				memoryGB = &f
			}
		}
	}

	if vcpu == nil {
		if n, err := extractVcpuFromText(rawJSON); err == nil {
			vcpu = &n
		}
	}
	if memoryGB == nil {
		if f, err := extractMemoryGBFromText(rawJSON); err == nil {
			memoryGB = &f
		}
	}

	return vcpu, memoryGB
}

func extractInstanceFamily(instanceType string) string {
	if instanceType == "" {
		return ""
	}
	if m := reInstToken.FindStringSubmatch(strings.ToLower(instanceType)); len(m) >= 1 {
		token := m[0]
		if idx := strings.IndexAny(token, ".-"); idx >= 0 {
			token = token[:idx]
		}
		return token
	}
	return ""
}

func nilToStrInt(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

func nilToStrFloat(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *p)
}

func nilToStrFloat6(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%.6f", *p)
}

func isLikelyCompute(productFamily string, attributes map[string]interface{}, instanceType string) bool {
	if strings.Contains(strings.ToLower(productFamily), "compute") {
		return true
	}
	if attributes != nil {
		for k := range attributes {
			kl := strings.ToLower(k)
			if strings.Contains(kl, "instance") || strings.Contains(kl, "vcpu") || strings.Contains(kl, "memory") {
				return true
			}
		}
	}
	return instanceType != ""
}

func extractInstanceToken(s string) string {
	if m := reInstToken.FindStringSubmatch(strings.ToLower(s)); len(m) >= 1 {
		return m[0]
	}
	return ""
}

func canonicalizeAwsRegion(loc string) string {
	r := strings.ToLower(strings.ReplaceAll(loc, " ", ""))
	r = strings.ReplaceAll(r, "(", "")
	r = strings.ReplaceAll(r, ")", "")
	return r
}

func extractAWSPriceFromTerms(terms map[string]interface{}) (*float64, string, string) {
	if terms == nil {
		return nil, "", ""
	}
	if on, ok := terms["OnDemand"]; ok {
		if odMap, ok := on.(map[string]interface{}); ok {
			for _, term := range odMap {
				if termMap, ok := term.(map[string]interface{}); ok {
					if pds, ok := termMap["priceDimensions"].(map[string]interface{}); ok {
						for _, pd := range pds {
							if pdMap, ok := pd.(map[string]interface{}); ok {
								if ppu, ok := pdMap["pricePerUnit"].(map[string]interface{}); ok {
									if usd, ok := ppu["USD"].(string); ok {
										if f, err := strconv.ParseFloat(usd, 64); err == nil {
											unit := getUnit(pdMap)
											return &f, "USD", unit
										}
									}
									if usdF, ok := ppu["USD"].(float64); ok {
										unit := getUnit(pdMap)
										return &usdF, "USD", unit
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return nil, "", ""
}

func getUnit(pdMap map[string]interface{}) string {
	if u, ok := pdMap["unit"].(string); ok {
		return u
	}
	return ""
}

func extractVcpuFromText(s string) (int, error) {
	if m := regexp.MustCompile(`(?i)"?vcpu"?\s*[:=]\s*"?([0-9]{1,4})"?`).FindStringSubmatch(s); len(m) >= 2 {
		if n, err := strconvAtoiSafe(m[1]); err == nil {
			return n, nil
		}
	}
	if m := regexp.MustCompile(`(?i)\(?\b([0-9]{1,3})\s*(v?cpu|vcpu|v-cpu|cores?)\b`).FindStringSubmatch(s); len(m) >= 2 {
		if n, err := strconvAtoiSafe(m[1]); err == nil {
			return n, nil
		}
	}
	return 0, errors.New("vcpu not found")
}

func extractMemoryGBFromText(s string) (float64, error) {
	if m := reAwsMemoryGB.FindStringSubmatch(s); len(m) >= 2 {
		if f, err := strconvParseFloatSafe(m[1]); err == nil {
			return f, nil
		}
	}
	return 0, errors.New("memory not found")
}

func parseMemoryString(s string) (float64, error) {
	if m := reAwsMemoryGB.FindStringSubmatch(s); len(m) >= 2 {
		if f, err := strconvParseFloatSafe(m[1]); err == nil {
			return f, nil
		}
	}
	return 0, fmt.Errorf("no memory in string")
}

func strconvAtoiSafe(s string) (int, error) {
	s = strings.TrimSpace(s)
	return strconv.Atoi(s)
}

func strconvParseFloatSafe(s string) (float64, error) {
	s = strings.TrimSpace(s)
	return strconv.ParseFloat(s, 64)
}
