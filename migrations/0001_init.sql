-- Enable UUID generation
create extension if not exists pgcrypto;

create table if not exists users (
  id uuid primary key default gen_random_uuid(),
  firebase_uid text not null unique,
  email text,
  display_name text,
  photo_url text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists projects (
  id uuid primary key default gen_random_uuid(),
  public_id text not null unique,           
  user_id uuid not null references users(id) on delete cascade,
  name text not null,
  is_temporary boolean not null default false,
  deleted_at timestamptz,
  current_diagram_version_id uuid,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists diagram_versions (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  version_number int not null,
  source text not null,                     
  diagram_json jsonb,                        
  image_object_key text,                    
  spec_summary jsonb,                        
  hash text,                                
  created_by uuid references users(id),
  created_at timestamptz not null default now(),
  unique(project_id, version_number)
);

create table if not exists chat_threads (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  title text,
  binding_mode text not null default 'FOLLOW_LATEST',  
  pinned_diagram_version_id uuid references diagram_versions(id),
  created_at timestamptz not null default now()
);

create table if not exists chat_messages (
  id uuid primary key default gen_random_uuid(),
  thread_id uuid not null references chat_threads(id) on delete cascade,
  project_id uuid not null references projects(id) on delete cascade,
  role text not null,                       
  content text not null,
  source text,                              
  refs jsonb,                               
  diagram_version_id_used uuid references diagram_versions(id),
  created_at timestamptz not null default now()
);

create index if not exists idx_projects_user on projects(user_id) where deleted_at is null;
create index if not exists idx_diagram_versions_project on diagram_versions(project_id, version_number desc);
create index if not exists idx_chat_messages_thread on chat_messages(thread_id, created_at);
