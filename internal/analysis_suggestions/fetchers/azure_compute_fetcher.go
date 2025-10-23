package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/time/rate"
)

type CloudComputePrice struct {
	ID           string                 `json:"id"`
	Provider     string                 `json:"provider"`
	SKUID        string                 `json:"sku_id"`
	Region       string                 `json:"region"`
	InstanceType string                 `json:"instance_type,omitempty"`
	VCPU         *int                   `json:"vcpu,omitempty"`
	MemoryGB     *float64               `json:"memory_gb,omitempty"`
	PricePerHour *float64               `json:"price_per_hour,omitempty"`
	Currency     string                 `json:"currency,omitempty"`
	Unit         string                 `json:"unit,omitempty"`
	FetchedAt    time.Time              `json:"fetched_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

var (
	reVCPU   = regexp.MustCompile(`(?i)(\b[0-9]{1,3})\s*(v?cpu|vcpu|v-cpu|cores?)\b`)
	reGiB    = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*(GiB|GB|gib|gb)\b`)
	reInst   = regexp.MustCompile(`(?i)(standard|d|e|f|m|n|b|c)[0-9a-z_\-\.]+`)
	reDigits = regexp.MustCompile(`\d+`)
)

var AzureVMSpecs = map[string]struct {
	VCPU     int
	MemoryGB float64
}{
	"Standard_D2s_v3":  {2, 8},
	"Standard_D4s_v3":  {4, 16},
	"Standard_D8s_v3":  {8, 32},
	"Standard_D16s_v3": {16, 64},
	"Standard_D32s_v3": {32, 128},
	"Standard_D64s_v3": {64, 256},

	"Standard_D2s_v4":  {2, 8},
	"Standard_D4s_v4":  {4, 16},
	"Standard_D8s_v4":  {8, 32},
	"Standard_D16s_v4": {16, 64},

	"Standard_E2s_v3":  {2, 16},
	"Standard_E4s_v3":  {4, 32},
	"Standard_E8s_v3":  {8, 64},
	"Standard_E16s_v3": {16, 128},
	"Standard_E32s_v3": {32, 256},
	"Standard_E64i_v3": {64, 432},

	"Standard_F2s_v2":  {2, 4},
	"Standard_F4s_v2":  {4, 8},
	"Standard_F8s_v2":  {8, 16},
	"Standard_F16s_v2": {16, 32},
	"Standard_F32s_v2": {32, 64},

	"Standard_B1s":  {1, 1},
	"Standard_B2s":  {2, 4},
	"Standard_B4ms": {4, 16},
	"Standard_B8ms": {8, 32},

	"Standard_M16ms": {16, 256},
	"Standard_M32ts": {32, 512},
	"Standard_M64s":  {64, 1024},

	"Standard_NC6":  {6, 56},
	"Standard_NC12": {12, 112},
}

func lookupVMSpec(s string) (vcpu *int, mem *float64, found bool) {
	if s == "" {
		return nil, nil, false
	}
	tries := []string{
		strings.TrimSpace(s),
		strings.ReplaceAll(strings.TrimSpace(s), " ", "_"),
		"Standard_" + strings.TrimSpace(s),
		"Standard_" + strings.ReplaceAll(strings.TrimSpace(s), " ", "_"),
		strings.ToUpper(strings.TrimSpace(s)),
		strings.Title(strings.TrimSpace(s)),
	}
	for _, t := range tries {
		t = strings.Trim(t, " ,()")
		if spec, ok := AzureVMSpecs[t]; ok {
			v := spec.VCPU
			m := spec.MemoryGB
			return &v, &m, true
		}
	}
	return nil, nil, false
}

func main() {
	outDir := "out/asm"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("failed to create out dir: %v", err)
	}
	csvPath := filepath.Join(outDir, "azure_compute_prices.csv")
	txtPath := filepath.Join(outDir, "azure_compute_prices.txt")

	ctx := context.Background()
	limiter := rate.NewLimiter(rate.Limit(5), 10)

	log.Printf("Starting Azure compute fetcher -> CSV: %s , TXT: %s\n", csvPath, txtPath)
	if err := fetchComputeAndWriteTables(ctx, limiter, 200, csvPath, txtPath); err != nil {
		log.Fatalf("fetch failed: %v", err)
	}
	log.Printf("Finished. Files: %s , %s\n", csvPath, txtPath)
}

func fetchComputeAndWriteTables(ctx context.Context, limiter *rate.Limiter, pageSize int, csvPath, txtPath string) error {
	csvF, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("create csv: %w", err)
	}
	defer csvF.Close()
	csvW := csv.NewWriter(csvF)
	header := []string{"id", "provider", "sku_id", "region", "instance_type", "vcpu", "memory_gb", "price_per_hour", "currency", "unit", "fetched_at"}
	if err := csvW.Write(header); err != nil {
		return fmt.Errorf("csv header write: %w", err)
	}

	txtF, err := os.Create(txtPath)
	if err != nil {
		return fmt.Errorf("create txt: %w", err)
	}
	defer txtF.Close()
	tw := tabwriter.NewWriter(bufio.NewWriter(txtF), 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
		"id", "provider", "sku_id", "region", "instance_type", "vcpu", "memory_gb", "price_per_hour", "currency", "unit", "fetched_at")
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush header: %w", err)
	}

	base := "https://prices.azure.com/api/retail/prices"
	filteredBase := base + "?$filter=serviceFamily%20eq%20'Compute'%20and%20(armRegionName%20eq%20'southeastasia')"

	skip := 0
	total := 0

	for {
		url := fmt.Sprintf("%s&$top=%d&$skip=%d", filteredBase, pageSize, skip)

		if err := limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter error: %w", err)
		}

		body, err := httpGetWithRetry(ctx, url)
		if err != nil {
			return fmt.Errorf("http get failed for %s: %w", url, err)
		}

		var page struct {
			Items []map[string]interface{} `json:"Items"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("json unmarshal failed for skip=%d: %w", skip, err)
		}
		if len(page.Items) == 0 {
			break
		}

		for _, item := range page.Items {
			cp := normalizeAzureComputeItem(item)
			if cp.PricePerHour == nil || cp.Region == "" {
				continue
			}
			cp.ID = fmt.Sprintf("%s|%s|%s", cp.Provider, cp.SKUID, cp.Region)
			cp.FetchedAt = time.Now().UTC()

			csvRow := make([]string, len(header))
			csvRow[0] = cp.ID
			csvRow[1] = cp.Provider
			csvRow[2] = cp.SKUID
			csvRow[3] = cp.Region
			csvRow[4] = cp.InstanceType
			if cp.VCPU != nil {
				csvRow[5] = strconv.Itoa(*cp.VCPU)
			} else {
				csvRow[5] = ""
			}
			if cp.MemoryGB != nil {
				csvRow[6] = fmt.Sprintf("%.2f", *cp.MemoryGB)
			} else {
				csvRow[6] = ""
			}
			if cp.PricePerHour != nil {
				csvRow[7] = fmt.Sprintf("%.6f", *cp.PricePerHour)
			} else {
				csvRow[7] = ""
			}
			csvRow[8] = cp.Currency
			csvRow[9] = cp.Unit
			csvRow[10] = cp.FetchedAt.Format(time.RFC3339Nano)

			if err := csvW.Write(csvRow); err != nil {
				return fmt.Errorf("csv write error: %w", err)
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				cp.ID,
				cp.Provider,
				cp.SKUID,
				cp.Region,
				cp.InstanceType,
				func() string {
					if cp.VCPU == nil {
						return ""
					}
					return strconv.Itoa(*cp.VCPU)
				}(),
				func() string {
					if cp.MemoryGB == nil {
						return ""
					}
					return fmt.Sprintf("%.2f", *cp.MemoryGB)
				}(),
				func() string {
					if cp.PricePerHour == nil {
						return ""
					}
					return fmt.Sprintf("%.6f", *cp.PricePerHour)
				}(),
				cp.Currency,
				cp.Unit,
				cp.FetchedAt.Format(time.RFC3339Nano),
			)

			total++
			if total%500 == 0 {
				csvW.Flush()
				if err := tw.Flush(); err != nil {
					return fmt.Errorf("txt flush error: %w", err)
				}
			}
		}

		skip += pageSize
		time.Sleep(120 * time.Millisecond)
	}

	csvW.Flush()
	if err := csvW.Error(); err != nil {
		return fmt.Errorf("csv final flush error: %w", err)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("txt final flush error: %w", err)
	}

	log.Printf("Wrote %d records to CSV/TXT", total)
	return nil
}

func normalizeAzureComputeItem(item map[string]interface{}) CloudComputePrice {
	cp := CloudComputePrice{
		Provider: "azure",
		Metadata: item,
		Unit:     "",
	}

	if v, ok := item["skuId"].(string); ok {
		cp.SKUID = v
	}
	var armSku string
	if v, ok := item["armSkuName"].(string); ok {
		armSku = v
		cp.InstanceType = armSku
		if vc, mem, ok := lookupVMSpec(armSku); ok {
			cp.VCPU = vc
			cp.MemoryGB = mem
		}
	}
	if v, ok := item["productName"].(string); ok {
		if cp.InstanceType == "" {
			cp.InstanceType = guessInstanceType(v)
		}
		if cp.VCPU == nil || cp.MemoryGB == nil {
			if vc, mem, ok := lookupVMSpec(v); ok {
				cp.VCPU = vc
				cp.MemoryGB = mem
			}
		}
	}

	if v, ok := item["armRegionName"].(string); ok {
		cp.Region = canonicalizeRegion(v)
	} else if v, ok := item["location"].(string); ok {
		cp.Region = canonicalizeRegion(v)
	}

	if v, ok := item["meterName"].(string); ok {
		tryParseFromStringFields(&cp, v)
		if cp.InstanceType == "" {
			parts := strings.Split(v, "/")
			if len(parts) > 0 {
				if vc, mem, ok := lookupVMSpec(parts[0]); ok {
					cp.VCPU = vc
					cp.MemoryGB = mem
				}
				if cp.InstanceType == "" {
					cp.InstanceType = strings.TrimSpace(parts[0])
				}
			}
		}
	}

	if v, ok := item["unitOfMeasure"].(string); ok {
		cp.Unit = v
	}

	if rp, ok := item["retailPrice"].(float64); ok {
		if strings.EqualFold(cp.Unit, "") {
			cp.Unit = "Hour"
		}
		price := rp
		cp.PricePerHour = &price
	} else if rpStr, ok := item["retailPrice"].(string); ok {
		if f, err := strconv.ParseFloat(rpStr, 64); err == nil {
			price := f
			cp.PricePerHour = &price
		}
	}

	if cur, ok := item["currencyCode"].(string); ok {
		cp.Currency = cur
	}

	if (cp.VCPU == nil || cp.MemoryGB == nil) && cp.InstanceType != "" {
		tryParseFromStringFields(&cp, cp.InstanceType)
	}
	if cp.VCPU == nil || cp.MemoryGB == nil {
		if v, ok := item["productName"].(string); ok {
			tryParseFromStringFields(&cp, v)
		}
	}

	return cp
}

func tryParseFromStringFields(cp *CloudComputePrice, s string) {
	if s == "" {
		return
	}
	if cp.VCPU == nil {
		if m := reVCPU.FindStringSubmatch(s); len(m) >= 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				cp.VCPU = &n
			}
		} else if m := regexp.MustCompile(`(?i)(\d+)\s*core`).FindStringSubmatch(s); len(m) >= 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				cp.VCPU = &n
			}
		} else {
			if m := reDigits.FindStringSubmatch(s); len(m) >= 1 {
				if n, err := strconv.Atoi(m[0]); err == nil && n <= 1024 {
					cp.VCPU = &n
				}
			}
		}
	}
	if cp.MemoryGB == nil {
		if m := reGiB.FindStringSubmatch(s); len(m) >= 2 {
			if f, err := strconv.ParseFloat(m[1], 64); err == nil {
				cp.MemoryGB = &f
			}
		}
	}
}

func guessInstanceType(s string) string {
	if m := reInst.FindString(s); len(m) > 0 {
		return strings.TrimSpace(m)
	}
	parts := strings.Fields(s)
	for _, p := range parts {
		if reDigits.MatchString(p) && len(p) <= 20 {
			return strings.Trim(p, ",()")
		}
	}
	return ""
}

func canonicalizeRegion(raw string) string {
	r := strings.TrimSpace(raw)
	r = strings.ToLower(r)
	r = strings.ReplaceAll(r, " ", "")
	r = strings.ReplaceAll(r, "(", "")
	r = strings.ReplaceAll(r, ")", "")
	r = strings.ReplaceAll(r, ".", "")
	r = strings.ReplaceAll(r, "/", "")
	return r
}

func httpGetWithRetry(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	maxAttempts := 6
	baseDelay := 400 * time.Millisecond

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "go-sim-azure-compute-fetcher/1.0")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else {
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return body, nil
				}
				lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
			}
		}
		sleep := baseDelay * time.Duration(1<<(attempt-1))
		jitter := time.Duration(rand.Int63n(int64(sleep / 2)))
		sleep = sleep + jitter
		if sleep > 8*time.Second {
			sleep = 8 * time.Second
		}
		log.Printf("Attempt %d failed for %s: %v — retrying in %s", attempt, url, lastErr, sleep)
		time.Sleep(sleep)
	}

	return nil, fmt.Errorf("all attempts failed: %v", lastErr)
}
