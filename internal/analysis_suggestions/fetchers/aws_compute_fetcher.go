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
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"golang.org/x/time/rate"
)

type CloudComputePrice struct {
	ID           string                 `json:"id"`
	Provider     string                 `json:"provider"`
	SKUID        string                 `json:"sku_id"`
	Region       string                 `json:"region"`
	InstanceType string                 `json:"instance_type"`
	VCPU         *int                   `json:"vcpu,omitempty"`
	MemoryGB     *float64               `json:"memory_gb,omitempty"`
	PricePerHour *float64               `json:"price_per_hour,omitempty"`
	Currency     string                 `json:"currency,omitempty"`
	Unit         string                 `json:"unit,omitempty"`
	FetchedAt    time.Time              `json:"fetched_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
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
		MaxRecords:     10000,
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

	header := []string{"id", "provider", "sku_id", "region", "instance_type", "vcpu", "memory_gb", "price_per_hour", "currency", "unit", "fetched_at"}
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

				line := fmt.Sprintf("%q,%q,%q,%q,%q,%q,%q,%q,%q,%q,%q\n",
					rec.ID, rec.Provider, rec.SKUID, rec.Region, rec.InstanceType,
					nilToStrInt(rec.VCPU), nilToStrFloat(rec.MemoryGB), nilToStrFloat6(rec.PricePerHour),
					rec.Currency, rec.Unit, rec.FetchedAt.Format(time.RFC3339Nano))
				if _, err := csvW.WriteString(line); err != nil {
					writerMutex.Unlock()
					errChan <- err
					return
				}

				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					rec.ID, rec.Provider, rec.SKUID, rec.Region, rec.InstanceType,
					nilToStrInt(rec.VCPU), nilToStrFloat(rec.MemoryGB), nilToStrFloat6(rec.PricePerHour),
					rec.Currency, rec.Unit, rec.FetchedAt.Format(time.RFC3339Nano))

				writerMutex.Unlock()
			}
		}()
	}

	input := &pricing.GetProductsInput{
		ServiceCode:   aws.String("AmazonEC2"),
		FormatVersion: aws.String("aws_v1"),
		Filters: []pricingtypes.Filter{
			{
				Field: aws.String("location"),
				Type:  pricingtypes.FilterTypeTermMatch,
				Value: aws.String("US East (N. Virginia)"),
			},
		},
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

			var instanceType string
			if attributes != nil {
				if v, ok := attributes["instanceType"].(string); ok {
					instanceType = v
				} else if v, ok := attributes["instanceFamily"].(string); ok {
					instanceType = v
				} else if v, ok := attributes["sku"].(string); ok && instanceType == "" {
					instanceType = v
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

			price, currency, unit := extractAWSPriceFromTerms(terms)
			if price == nil {
				continue
			}

			rec := &CloudComputePrice{
				ID:           fmt.Sprintf("aws|%s|%s", skuID, region),
				Provider:     "aws",
				SKUID:        skuID,
				Region:       region,
				InstanceType: instanceType,
				VCPU:         vcpu,
				MemoryGB:     memoryGB,
				PricePerHour: price,
				Currency:     currency,
				Unit:         unit,
				FetchedAt:    time.Now().UTC(),
			}

			recordChan <- rec
			total++
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

func writeCSVLine(f *os.File, r CloudComputePrice) error {
	line := fmt.Sprintf("%q,%q,%q,%q,%q,%q,%q,%q,%q,%q,%q\n",
		r.ID, r.Provider, r.SKUID, r.Region, r.InstanceType,
		nilToStrInt(r.VCPU), nilToStrFloat(r.MemoryGB), nilToStrFloat6(r.PricePerHour),
		r.Currency, r.Unit, r.FetchedAt.Format(time.RFC3339Nano))
	_, err := f.WriteString(line)
	return err
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
