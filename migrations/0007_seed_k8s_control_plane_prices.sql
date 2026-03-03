CREATE TABLE k8s_control_plane_prices (
    id              TEXT PRIMARY KEY,
    provider        TEXT NOT NULL,      
    service         TEXT NOT NULL,      
    tier            TEXT NOT NULL,   
    region_scope    TEXT NOT NULL,      
    price_per_hour  NUMERIC NOT NULL,
    currency        TEXT NOT NULL,
    unit            TEXT NOT NULL,
    source          TEXT NOT NULL,
    effective_from  DATE NOT NULL
    );

CREATE INDEX idx_k8s_cp_lookup
ON k8s_control_plane_prices (provider, service, tier);


INSERT INTO k8s_control_plane_prices VALUES
('aws|eks|standard', 'aws', 'eks', 'standard', 'global', 0.10, 'USD', 'Hrs', 'aws-docs', '2025-12-10' ),
('aws|eks|extended', 'aws', 'eks', 'extended', 'global', 0.60, 'USD', 'Hrs', 'aws-docs', '2025-12-10' );


INSERT INTO k8s_control_plane_prices VALUES
('azure|aks|free',     'azure', 'aks', 'free',     'global', 0.00, 'USD', 'Hrs', 'azure-docs', '2025-12-10' ),
('azure|aks|standard', 'azure', 'aks', 'standard', 'global', 0.10, 'USD', 'Hrs', 'azure-docs', '2025-12-10' ),
('azure|aks|premium',  'azure', 'aks', 'premium',  'global', 0.60, 'USD', 'Hrs', 'azure-docs', '2025-12-10' );


INSERT INTO k8s_control_plane_prices VALUES
('gcp|gke|standard',   'gcp', 'gke', 'standard',  'global', 0.10, 'USD', 'Hrs', 'gcp-docs', '2025-12-10'),
('gcp|gke|autopilot',  'gcp', 'gke', 'autopilot', 'global', 0.10, 'USD', 'Hrs', 'gcp-docs', '2025-12-10');

INSERT INTO k8s_control_plane_prices VALUES
('aws|eks|pctl|xl',  'aws', 'eks', 'pctl_xl',  'global', 1.65, 'USD', 'Hrs', 'aws-docs', '2025-12-10'  ),
('aws|eks|pctl|2xl', 'aws', 'eks', 'pctl_2xl', 'global', 3.40, 'USD', 'Hrs', 'aws-docs', '2025-12-10'  ),
('aws|eks|pctl|4xl', 'aws', 'eks', 'pctl_4xl', 'global', 6.90, 'USD', 'Hrs', 'aws-docs', '2025-12-10'  );
