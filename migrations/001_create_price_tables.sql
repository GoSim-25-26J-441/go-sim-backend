
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS azure_compute_prices (
  id                   uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  sku_id               text NOT NULL,
  provider             text NOT NULL DEFAULT 'azure',
  region               text NOT NULL,
  instance_type        text,
  vcpu                 integer,
  memory_gb            double precision,
  price_per_hour       double precision,
  currency             text,
  unit                 text,
  service_family       text,
  purchase_option      text,
  lease_contract_length text,
  fetched_at           timestamptz NOT NULL DEFAULT now(),
  metadata             jsonb,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (sku_id, region)
);

CREATE INDEX IF NOT EXISTS idx_azure_region ON azure_compute_prices(region);
CREATE INDEX IF NOT EXISTS idx_azure_sku ON azure_compute_prices(sku_id);
CREATE INDEX IF NOT EXISTS idx_azure_purchase_option ON azure_compute_prices(purchase_option);
CREATE INDEX IF NOT EXISTS idx_azure_service_family ON azure_compute_prices(service_family);
CREATE INDEX IF NOT EXISTS idx_azure_instance_type ON azure_compute_prices(instance_type);

CREATE TABLE IF NOT EXISTS gcp_compute_prices (
  id              uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  sku_id          text NOT NULL,
  provider        text NOT NULL DEFAULT 'gcp',
  region          text NOT NULL,
  instance_type   text,
  resource_family text,
  vcpu            integer,
  memory_gb       double precision,
  price_per_hour  double precision,
  currency        text,
  unit            text,
  purchase_option text,
  usage_type      text,
  fetched_at      timestamptz NOT NULL DEFAULT now(),
  metadata        jsonb,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (sku_id, region)
);

CREATE INDEX IF NOT EXISTS idx_gcp_region ON gcp_compute_prices(region);
CREATE INDEX IF NOT EXISTS idx_gcp_sku ON gcp_compute_prices(sku_id);
CREATE INDEX IF NOT EXISTS idx_gcp_purchase_option ON gcp_compute_prices(purchase_option);
CREATE INDEX IF NOT EXISTS idx_gcp_resource_family ON gcp_compute_prices(resource_family);
CREATE INDEX IF NOT EXISTS idx_gcp_instance_type ON gcp_compute_prices(instance_type);


CREATE TABLE IF NOT EXISTS aws_compute_prices (
  id                     text PRIMARY KEY,
  provider               text NOT NULL DEFAULT 'aws',
  sku_id                 text NOT NULL,
  region                 text NOT NULL,
  instance_type          text,
  instance_family        text,
  vcpu                   integer,
  memory_gb              double precision,
  price_per_hour         double precision,
  currency               text,
  unit                   text,
  purchase_option        text,  
  lease_contract_length  text, 
  fetched_at             timestamptz NOT NULL DEFAULT now(),
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now(),

  UNIQUE (sku_id, region, purchase_option, lease_contract_length)
);

CREATE INDEX IF NOT EXISTS idx_aws_region ON aws_compute_prices(region);
CREATE INDEX IF NOT EXISTS idx_aws_sku ON aws_compute_prices(sku_id);
CREATE INDEX IF NOT EXISTS idx_aws_purchase_option ON aws_compute_prices(purchase_option);
CREATE INDEX IF NOT EXISTS idx_aws_instance_type ON aws_compute_prices(instance_type);
CREATE INDEX IF NOT EXISTS idx_aws_instance_family ON aws_compute_prices(instance_family);
