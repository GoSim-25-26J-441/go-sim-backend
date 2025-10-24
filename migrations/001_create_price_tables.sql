
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS azure_compute_prices (
  id            uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  sku_id        text NOT NULL,
  provider      text NOT NULL DEFAULT 'azure',
  region        text NOT NULL,
  instance_type text,
  vcpu          integer,
  memory_gb     double precision,
  price_per_hour double precision,
  currency      text,
  unit          text,
  fetched_at    timestamptz NOT NULL DEFAULT now(),
  metadata      jsonb,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (sku_id, region)
);

CREATE INDEX IF NOT EXISTS idx_azure_region ON azure_compute_prices(region);
CREATE INDEX IF NOT EXISTS idx_azure_sku ON azure_compute_prices(sku_id);

CREATE TABLE IF NOT EXISTS gcp_compute_prices (
  id            uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  sku_id        text NOT NULL,
  provider      text NOT NULL DEFAULT 'gcp',
  region        text NOT NULL,
  instance_type text,
  description   text,
  vcpu          integer,
  memory_gb     double precision,
  price_per_hour double precision,
  currency      text,
  unit          text,
  fetched_at    timestamptz NOT NULL DEFAULT now(),
  metadata      jsonb,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (sku_id, region)
);

CREATE INDEX IF NOT EXISTS idx_gcp_region ON gcp_compute_prices(region);
CREATE INDEX IF NOT EXISTS idx_gcp_sku ON gcp_compute_prices(sku_id);


CREATE TABLE IF NOT EXISTS aws_compute_prices (
  id            uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  sku_id        text NOT NULL,
  provider      text NOT NULL DEFAULT 'aws',
  region        text NOT NULL,
  instance_type text,
  vcpu          integer,
  memory_gb     double precision,
  price_per_hour double precision,
  currency      text,
  unit          text,
  fetched_at    timestamptz NOT NULL DEFAULT now(),
  metadata      jsonb,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (sku_id, region)
);

CREATE INDEX IF NOT EXISTS idx_aws_region ON aws_compute_prices(region);
CREATE INDEX IF NOT EXISTS idx_aws_sku ON aws_compute_prices(sku_id);
