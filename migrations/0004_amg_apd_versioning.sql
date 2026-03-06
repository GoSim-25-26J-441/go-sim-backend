-- AMG-APD versioning is stored in the unified diagram_versions table
-- defined in 0002_projects_chats_diagrams.sql. This migration drops the
-- legacy amg_apd_versions table if it exists (e.g. from an older deployment).
-- New installs use only diagram_versions with source = 'amg_apd'.

DROP TABLE IF EXISTS amg_apd_versions;
