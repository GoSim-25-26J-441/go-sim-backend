package domain

import "time"

// AzurePriceRow represents a row for Azure compute prices
type AzurePriceRow struct {
	SKUID        string
	Region       string
	InstanceType string
	VCPU         *int
	MemoryGB     *float64
	PricePerHour *float64
	Currency     string
	Unit         string
	FetchedAt    time.Time
	Metadata     map[string]interface{}
}

// GCPPriceRow represents a row for GCP compute prices
type GCPPriceRow struct {
	SKUID        string
	Region       string
	Description  string
	VCPU         *int
	MemoryGB     *float64
	PricePerUnit *float64
	Currency     string
	Unit         string
	FetchedAt    time.Time
	Metadata     map[string]interface{}
}

// AWSPriceRow represents a row for AWS compute prices
type AWSPriceRow struct {
	SKUID        string
	Region       string
	InstanceType string
	VCPU         *int
	MemoryGB     *float64
	PricePerHour *float64
	Currency     string
	Unit         string
	FetchedAt    time.Time
	Metadata     map[string]interface{}
}
