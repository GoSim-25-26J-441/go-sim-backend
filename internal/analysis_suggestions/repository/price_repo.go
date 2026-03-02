package repository

import (
	"context"
	"encoding/json"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func UpsertAzure(ctx context.Context, pool *pgxpool.Pool, r domain.AzurePriceRow) error {
	const sql = `
INSERT INTO azure_compute_prices
  (sku_id, provider, region, instance_type, vcpu, memory_gb, price_per_hour, currency, unit, fetched_at, metadata, created_at, updated_at)
VALUES ($1, 'azure', $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
ON CONFLICT (sku_id, region) DO UPDATE
  SET instance_type = EXCLUDED.instance_type,
      vcpu = EXCLUDED.vcpu,
      memory_gb = EXCLUDED.memory_gb,
      price_per_hour = EXCLUDED.price_per_hour,
      currency = EXCLUDED.currency,
      unit = EXCLUDED.unit,
      fetched_at = EXCLUDED.fetched_at,
      metadata = EXCLUDED.metadata,
      updated_at = now()
;`
	metaJSON, _ := json.Marshal(r.Metadata)
	_, err := pool.Exec(ctx, sql,
		r.SKUID, r.Region, r.InstanceType, r.VCPU, r.MemoryGB, r.PricePerHour,
		r.Currency, r.Unit, r.FetchedAt.UTC(), metaJSON,
	)
	return err
}

func UpsertAzureBatch(ctx context.Context, pool *pgxpool.Pool, rows []domain.AzurePriceRow) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	const sql = `
INSERT INTO azure_compute_prices
  (sku_id, provider, region, instance_type, vcpu, memory_gb, price_per_hour, currency, unit, fetched_at, metadata, created_at, updated_at)
VALUES ($1, 'azure', $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
ON CONFLICT (sku_id, region) DO UPDATE
  SET instance_type = EXCLUDED.instance_type,
      vcpu = EXCLUDED.vcpu,
      memory_gb = EXCLUDED.memory_gb,
      price_per_hour = EXCLUDED.price_per_hour,
      currency = EXCLUDED.currency,
      unit = EXCLUDED.unit,
      fetched_at = EXCLUDED.fetched_at,
      metadata = EXCLUDED.metadata,
      updated_at = now()
;`
	for _, r := range rows {
		metaJSON, _ := json.Marshal(r.Metadata)
		if _, err := tx.Exec(ctx, sql,
			r.SKUID, r.Region, r.InstanceType, r.VCPU, r.MemoryGB, r.PricePerHour,
			r.Currency, r.Unit, r.FetchedAt.UTC(), metaJSON,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func UpsertGCP(ctx context.Context, pool *pgxpool.Pool, r domain.GCPPriceRow) error {
	const sql = `
INSERT INTO gcp_compute_prices
  (sku_id, provider, region, description, vcpu, memory_gb, price_per_hour, currency, unit, fetched_at, metadata, created_at, updated_at)
VALUES ($1, 'gcp', $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
ON CONFLICT (sku_id, region) DO UPDATE
  SET description = EXCLUDED.description,
      vcpu = EXCLUDED.vcpu,
      memory_gb = EXCLUDED.memory_gb,
      price_per_hour = EXCLUDED.price_per_hour,
      currency = EXCLUDED.currency,
      unit = EXCLUDED.unit,
      fetched_at = EXCLUDED.fetched_at,
      metadata = EXCLUDED.metadata,
      updated_at = now()
;`
	meta, _ := json.Marshal(r.Metadata)
	_, err := pool.Exec(ctx, sql,
		r.SKUID, r.Region, r.Description, r.VCPU, r.MemoryGB, r.PricePerUnit,
		r.Currency, r.Unit, r.FetchedAt.UTC(), meta,
	)
	return err
}

func UpsertGCPBatch(ctx context.Context, pool *pgxpool.Pool, rows []domain.GCPPriceRow) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	const sql = `
INSERT INTO gcp_compute_prices
  (sku_id, provider, region, description, vcpu, memory_gb, price_per_hour, currency, unit, fetched_at, metadata, created_at, updated_at)
VALUES ($1, 'gcp', $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
ON CONFLICT (sku_id, region) DO UPDATE
  SET description = EXCLUDED.description,
      vcpu = EXCLUDED.vcpu,
      memory_gb = EXCLUDED.memory_gb,
      price_per_hour = EXCLUDED.price_per_hour,
      currency = EXCLUDED.currency,
      unit = EXCLUDED.unit,
      fetched_at = EXCLUDED.fetched_at,
      metadata = EXCLUDED.metadata,
      updated_at = now()
;`
	for _, r := range rows {
		meta, _ := json.Marshal(r.Metadata)
		if _, err := tx.Exec(ctx, sql,
			r.SKUID, r.Region, r.Description, r.VCPU, r.MemoryGB, r.PricePerUnit,
			r.Currency, r.Unit, r.FetchedAt.UTC(), meta,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func UpsertAWS(ctx context.Context, pool *pgxpool.Pool, r domain.AWSPriceRow) error {
	const sql = `
INSERT INTO aws_compute_prices
  (sku_id, provider, region, instance_type, vcpu, memory_gb, price_per_hour, currency, unit, fetched_at, metadata, created_at, updated_at)
VALUES ($1, 'aws', $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
ON CONFLICT (sku_id, region) DO UPDATE
  SET instance_type = EXCLUDED.instance_type,
      vcpu = EXCLUDED.vcpu,
      memory_gb = EXCLUDED.memory_gb,
      price_per_hour = EXCLUDED.price_per_hour,
      currency = EXCLUDED.currency,
      unit = EXCLUDED.unit,
      fetched_at = EXCLUDED.fetched_at,
      metadata = EXCLUDED.metadata,
      updated_at = now()
;`
	meta, _ := json.Marshal(r.Metadata)
	_, err := pool.Exec(ctx, sql,
		r.SKUID, r.Region, r.InstanceType, r.VCPU, r.MemoryGB, r.PricePerHour,
		r.Currency, r.Unit, r.FetchedAt.UTC(), meta,
	)
	return err
}

func UpsertAWSBatch(ctx context.Context, pool *pgxpool.Pool, rows []domain.AWSPriceRow) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	const sql = `
INSERT INTO aws_compute_prices
  (sku_id, provider, region, instance_type, vcpu, memory_gb, price_per_hour, currency, unit, fetched_at, metadata, created_at, updated_at)
VALUES ($1, 'aws', $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
ON CONFLICT (sku_id, region) DO UPDATE
  SET instance_type = EXCLUDED.instance_type,
      vcpu = EXCLUDED.vcpu,
      memory_gb = EXCLUDED.memory_gb,
      price_per_hour = EXCLUDED.price_per_hour,
      currency = EXCLUDED.currency,
      unit = EXCLUDED.unit,
      fetched_at = EXCLUDED.fetched_at,
      metadata = EXCLUDED.metadata,
      updated_at = now()
;`
	for _, r := range rows {
		meta, _ := json.Marshal(r.Metadata)
		if _, err := tx.Exec(ctx, sql,
			r.SKUID, r.Region, r.InstanceType, r.VCPU, r.MemoryGB, r.PricePerHour,
			r.Currency, r.Unit, r.FetchedAt.UTC(), meta,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
