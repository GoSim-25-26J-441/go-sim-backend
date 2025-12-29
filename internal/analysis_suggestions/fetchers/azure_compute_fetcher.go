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
	ID                  string                 `json:"id"`
	Provider            string                 `json:"provider"`
	SKUID               string                 `json:"sku_id"`
	Region              string                 `json:"region"`
	InstanceType        string                 `json:"instance_type,omitempty"`
	VCPU                *int                   `json:"vcpu,omitempty"`
	MemoryGB            *float64               `json:"memory_gb,omitempty"`
	PricePerHour        *float64               `json:"price_per_hour,omitempty"`
	Currency            string                 `json:"currency,omitempty"`
	Unit                string                 `json:"unit,omitempty"`
	ServiceFamily       string                 `json:"service_family,omitempty"`
	PurchaseOption      string                 `json:"purchase_option,omitempty"`
	LeaseContractLength string                 `json:"lease_contract_length,omitempty"`
	FetchedAt           time.Time              `json:"fetched_at"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
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
	// D-series v3
	"Standard_D2s_v3":  {2, 8},
	"Standard_D4s_v3":  {4, 16},
	"Standard_D8s_v3":  {8, 32},
	"Standard_D16s_v3": {16, 64},
	"Standard_D32s_v3": {32, 128},
	"Standard_D64s_v3": {64, 256},

	// D-series v4
	"Standard_D2s_v4":  {2, 8},
	"Standard_D4s_v4":  {4, 16},
	"Standard_D8s_v4":  {8, 32},
	"Standard_D16s_v4": {16, 64},
	"Standard_D32s_v4": {32, 128},
	"Standard_D48s_v4": {48, 192},
	"Standard_D64s_v4": {64, 256},

	// D-series v5
	"Standard_D2s_v5":  {2, 8},
	"Standard_D4s_v5":  {4, 16},
	"Standard_D8s_v5":  {8, 32},
	"Standard_D16s_v5": {16, 64},
	"Standard_D32s_v5": {32, 128},
	"Standard_D48s_v5": {48, 192},
	"Standard_D64s_v5": {64, 256},

	// Dds-series
	"Standard_D2ds_v4": {2, 8},
	"Standard_D4ds_v4": {4, 16},
	"Standard_D8ds_v4": {8, 32},

	// E-series v3
	"Standard_E2s_v3":   {2, 16},
	"Standard_E4s_v3":   {4, 32},
	"Standard_E8s_v3":   {8, 64},
	"Standard_E16s_v3":  {16, 128},
	"Standard_E32s_v3":  {32, 256},
	"Standard_E64i_v3":  {64, 432},
	"Standard_E64is_v3": {64, 432},

	// E-series v4
	"Standard_E2s_v4":  {2, 16},
	"Standard_E4s_v4":  {4, 32},
	"Standard_E8s_v4":  {8, 64},
	"Standard_E16s_v4": {16, 128},
	"Standard_E32s_v4": {32, 256},
	"Standard_E48s_v4": {48, 384},
	"Standard_E64s_v4": {64, 504},

	// E-series v5
	"Standard_E2s_v5":  {2, 16},
	"Standard_E4s_v5":  {4, 32},
	"Standard_E8s_v5":  {8, 64},
	"Standard_E16s_v5": {16, 128},
	"Standard_E32s_v5": {32, 256},
	"Standard_E48s_v5": {48, 384},
	"Standard_E64s_v5": {64, 504},

	// F-series v2
	"Standard_F2s_v2":  {2, 4},
	"Standard_F4s_v2":  {4, 8},
	"Standard_F8s_v2":  {8, 16},
	"Standard_F16s_v2": {16, 32},
	"Standard_F32s_v2": {32, 64},
	"Standard_F48s_v2": {48, 96},
	"Standard_F64s_v2": {64, 128},
	"Standard_F72s_v2": {72, 144},

	// B-series
	"Standard_B1s":  {1, 1},
	"Standard_B1ms": {1, 2},
	"Standard_B2s":  {2, 4},
	"Standard_B2ms": {2, 8},
	"Standard_B4ms": {4, 16},
	"Standard_B8ms": {8, 32},

	// M-series
	"Standard_M8ms":   {8, 224},
	"Standard_M16ms":  {16, 256},
	"Standard_M32ts":  {32, 512},
	"Standard_M32ls":  {32, 256},
	"Standard_M32ms":  {32, 512},
	"Standard_M64s":   {64, 1024},
	"Standard_M64ls":  {64, 512},
	"Standard_M64ms":  {64, 1792},
	"Standard_M128s":  {128, 2048},
	"Standard_M128ms": {128, 3892},

	// Ls-series
	"Standard_L4s":  {4, 32},
	"Standard_L8s":  {8, 64},
	"Standard_L16s": {16, 128},
	"Standard_L32s": {32, 256},
	"Standard_L48s": {48, 384},
	"Standard_L64s": {64, 512},

	// Lsv2-series
	"Standard_L8s_v2":  {8, 64},
	"Standard_L16s_v2": {16, 128},
	"Standard_L32s_v2": {32, 256},
	"Standard_L48s_v2": {48, 384},
	"Standard_L64s_v2": {64, 512},
	"Standard_L80s_v2": {80, 640},

	// Av2-series
	"Standard_A1_v2":  {1, 2},
	"Standard_A2_v2":  {2, 4},
	"Standard_A4_v2":  {4, 8},
	"Standard_A8_v2":  {8, 16},
	"Standard_A2m_v2": {2, 16},
	"Standard_A4m_v2": {4, 32},
	"Standard_A8m_v2": {8, 64},

	// G-series
	"Standard_G1": {2, 28},
	"Standard_G2": {4, 56},
	"Standard_G3": {8, 112},
	"Standard_G4": {16, 224},
	"Standard_G5": {32, 448},

	// X-series
	"Standard_E4-2s_v3":   {4, 32},
	"Standard_E8-2s_v3":   {8, 64},
	"Standard_E16-4s_v3":  {16, 128},
	"Standard_E20s_v3":    {20, 160},
	"Standard_E32-8s_v3":  {32, 256},
	"Standard_E32-16s_v3": {32, 256},

	// Dasv4-series
	"Standard_D2as_v4":  {2, 8},
	"Standard_D4as_v4":  {4, 16},
	"Standard_D8as_v4":  {8, 32},
	"Standard_D16as_v4": {16, 64},
	"Standard_D32as_v4": {32, 128},
	"Standard_D48as_v4": {48, 192},
	"Standard_D64as_v4": {64, 256},
	"Standard_D96as_v4": {96, 384},

	// Easv4-series
	"Standard_E2as_v4":  {2, 16},
	"Standard_E4as_v4":  {4, 32},
	"Standard_E8as_v4":  {8, 64},
	"Standard_E16as_v4": {16, 128},
	"Standard_E32as_v4": {32, 256},
	"Standard_E48as_v4": {48, 384},
	"Standard_E64as_v4": {64, 504},
	"Standard_E96as_v4": {96, 672},

	// Dasv5-series
	"Standard_D2as_v5":  {2, 8},
	"Standard_D4as_v5":  {4, 16},
	"Standard_D8as_v5":  {8, 32},
	"Standard_D16as_v5": {16, 64},
	"Standard_D32as_v5": {32, 128},
	"Standard_D48as_v5": {48, 192},
	"Standard_D64as_v5": {64, 256},
	"Standard_D96as_v5": {96, 384},

	// Easv5-series
	"Standard_E2as_v5":  {2, 16},
	"Standard_E4as_v5":  {4, 32},
	"Standard_E8as_v5":  {8, 64},
	"Standard_E16as_v5": {16, 128},
	"Standard_E32as_v5": {32, 256},
	"Standard_E48as_v5": {48, 384},
	"Standard_E64as_v5": {64, 504},
	"Standard_E96as_v5": {96, 672},

	// DC-series
	"Standard_DC2s": {2, 8},
	"Standard_DC4s": {4, 16},

	// Dpd-series
	"Standard_D2pd_v5":  {2, 8},
	"Standard_D4pd_v5":  {4, 16},
	"Standard_D8pd_v5":  {8, 32},
	"Standard_D16pd_v5": {16, 64},
	"Standard_D32pd_v5": {32, 128},
	"Standard_D48pd_v5": {48, 192},
	"Standard_D64pd_v5": {64, 256},
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

	maxRecords := 800000
	if err := fetchComputeAndWriteTables(ctx, limiter, 200, maxRecords, csvPath, txtPath); err != nil {
		log.Fatalf("fetch failed: %v", err)
	}
	log.Printf("Finished. Files: %s , %s\n", csvPath, txtPath)
}

func fetchComputeAndWriteTables(ctx context.Context, limiter *rate.Limiter, pageSize, maxRecords int, csvPath, txtPath string) error {
	csvF, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("create csv: %w", err)
	}
	defer csvF.Close()
	csvW := csv.NewWriter(csvF)

	// Updated header with new fields
	header := []string{
		"id", "provider", "sku_id", "region", "instance_type",
		"vcpu", "memory_gb", "price_per_hour", "currency", "unit",
		"service_family", "purchase_option", "lease_contract_length", "fetched_at",
	}
	if err := csvW.Write(header); err != nil {
		return fmt.Errorf("csv header write: %w", err)
	}

	txtF, err := os.Create(txtPath)
	if err != nil {
		return fmt.Errorf("create txt: %w", err)
	}
	defer txtF.Close()
	tw := tabwriter.NewWriter(bufio.NewWriter(txtF), 0, 4, 2, ' ', 0)

	// Header with fields
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
		"id", "provider", "sku_id", "region", "instance_type",
		"vcpu", "memory_gb", "price_per_hour", "currency", "unit",
		"service_family", "purchase_option", "lease_contract_length", "fetched_at")
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush header: %w", err)
	}

	base := "https://prices.azure.com/api/retail/prices"
	filteredBase := base + "?$filter=serviceFamily%20eq%20%27Compute%27"

	skip := 0
	total := 0

	for total < maxRecords {
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
			if total >= maxRecords {
				break
			}

			cp := normalizeAzureComputeItem(item)
			if cp.PricePerHour == nil || cp.Region == "" {
				continue
			}
			cp.ID = fmt.Sprintf("%s|%s|%s", cp.Provider, cp.SKUID, cp.Region)
			cp.FetchedAt = time.Now().UTC()

			// Build CSV row
			csvRow := make([]string, len(header))
			csvRow[0] = cp.ID
			csvRow[1] = cp.Provider
			csvRow[2] = cp.SKUID
			csvRow[3] = cp.Region
			csvRow[4] = cp.InstanceType
			csvRow[5] = formatOptionalInt(cp.VCPU)
			csvRow[6] = formatOptionalFloat(cp.MemoryGB, "%.2f")
			csvRow[7] = formatOptionalFloat(cp.PricePerHour, "%.6f")
			csvRow[8] = cp.Currency
			csvRow[9] = cp.Unit
			csvRow[10] = cp.ServiceFamily
			csvRow[11] = cp.PurchaseOption
			csvRow[12] = cp.LeaseContractLength
			csvRow[13] = cp.FetchedAt.Format(time.RFC3339Nano)

			if err := csvW.Write(csvRow); err != nil {
				return fmt.Errorf("csv write error: %w", err)
			}

			// Write to text file
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				cp.ID,
				cp.Provider,
				cp.SKUID,
				cp.Region,
				cp.InstanceType,
				formatOptionalInt(cp.VCPU),
				formatOptionalFloat(cp.MemoryGB, "%.2f"),
				formatOptionalFloat(cp.PricePerHour, "%.6f"),
				cp.Currency,
				cp.Unit,
				cp.ServiceFamily,
				cp.PurchaseOption,
				cp.LeaseContractLength,
				cp.FetchedAt.Format(time.RFC3339Nano),
			)

			total++

			if total%1000 == 0 {
				log.Printf("Processed %d/%d records (%.1f%%)", total, maxRecords, float64(total)/float64(maxRecords)*100)
			}

			if total%500 == 0 {
				csvW.Flush()
				if err := tw.Flush(); err != nil {
					return fmt.Errorf("txt flush error: %w", err)
				}
			}
		}

		if total >= maxRecords {
			log.Printf("Reached maximum record limit of %d", maxRecords)
			break
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

	log.Printf("Wrote %d records to CSV/TXT (max limit: %d)", total, maxRecords)
	return nil
}

func normalizeAzureComputeItem(item map[string]interface{}) CloudComputePrice {
	cp := CloudComputePrice{
		Provider: "azure",
		Metadata: item,
		Unit:     "",
	}

	// Extract basic fields
	if v, ok := item["skuId"].(string); ok {
		cp.SKUID = v
	}

	// Extract Service Family
	if v, ok := item["serviceFamily"].(string); ok {
		cp.ServiceFamily = v
	}

	// Extract Purchase Option (reservation terms)
	cp.PurchaseOption = "OnDemand" // default
	if v, ok := item["type"].(string); ok {
		if strings.EqualFold(v, "Consumption") {
			cp.PurchaseOption = "OnDemand"
		} else if strings.EqualFold(v, "Reservation") {
			cp.PurchaseOption = "Reserved"
		}
	}

	// Extract Lease Contract Length
	if v, ok := item["reservationTerm"].(string); ok {
		cp.LeaseContractLength = normalizeLeaseContract(v)
	}

	// Extract instance type and specs
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

	// Extract region
	if v, ok := item["armRegionName"].(string); ok {
		cp.Region = canonicalizeRegion(v)
	} else if v, ok := item["location"].(string); ok {
		cp.Region = canonicalizeRegion(v)
	}

	// Extract meter information for additional parsing
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

	// Extract unit
	if v, ok := item["unitOfMeasure"].(string); ok {
		cp.Unit = v
	}

	// Extract price
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

	// Extract currency
	if cur, ok := item["currencyCode"].(string); ok {
		cp.Currency = cur
	}

	// Fallback parsing for CPU and Memory
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

func normalizeLeaseContract(term string) string {
	term = strings.ToLower(strings.TrimSpace(term))

	switch {
	case strings.Contains(term, "1 year") || strings.Contains(term, "12 month"):
		return "1 Year"
	case strings.Contains(term, "3 year") || strings.Contains(term, "36 month"):
		return "3 Year"
	case strings.Contains(term, "5 year") || strings.Contains(term, "60 month"):
		return "5 Year"
	case term == "":
		return "" // For OnDemand
	default:
		// Try to extract any numeric pattern
		if re := regexp.MustCompile(`(\d+)\s*(year|month)`).FindStringSubmatch(term); len(re) >= 3 {
			value := re[1]
			unit := re[2]
			if unit == "month" {
				months, _ := strconv.Atoi(value)
				if months == 12 {
					return "1 Year"
				} else if months == 36 {
					return "3 Year"
				} else if months == 60 {
					return "5 Year"
				}
				return fmt.Sprintf("%d Months", months)
			}
			return fmt.Sprintf("%s Years", value)
		}
		return term
	}
}

func formatOptionalInt(val *int) string {
	if val == nil {
		return ""
	}
	return strconv.Itoa(*val)
}

func formatOptionalFloat(val *float64, format string) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf(format, *val)
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
