create table if not exists people (
    id integer primary key,
    name text,
    date_of_birth date,
    date_of_death date,
    popularity real default 0,
    fetched boolean default false,
    created_at timestamp default now(),
    updated_at timestamp default now()
);

create index if not exists idx_people_fetched on people(fetched);

create index if not exists idx_people_dates_covering on people (id) include (date_of_birth, date_of_death, popularity, fetched);

create table if not exists sync_job_status (
    id serial primary key,
    type text not null,
    start_date date not null,
    end_date date not null,
    status text not null default 'running',
    message text,
    updated_ids integer[],
    tmdb_change_count integer default 0,
    updated_count integer default 0,
    duration_ms integer,
    started_at timestamp default now(),
    finished_at timestamp,
    created_at timestamp default now(),
    updated_at timestamp default now()
);

create table if not exists season_cast_cache (
    series_id     int not null,
    season_number int not null,
    cast_data     jsonb not null,
    expires_at    timestamptz not null,
    created_at    timestamptz not null default now(),
    primary key (series_id, season_number)
);

create index if not exists idx_season_cast_cache_expires on season_cast_cache (expires_at);

create table if not exists interesting_info (
    entity_type text not null,
    entity_id   integer not null,
    name        text not null,
    data        jsonb not null,
    fetched_at  timestamptz not null default now(),
    created_at  timestamptz not null default now(),
    updated_at  timestamptz not null default now(),
    primary key (entity_type, entity_id)
);

-- ============================================================
-- Auth & User Management
-- ============================================================

create table if not exists users (
    id uuid primary key default gen_random_uuid(),
    username text unique,
    given_name text,
    avatar_url text,
    last_exported_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create unique index if not exists idx_users_username_lower
    on users (lower(username))
    where username is not null;

create table if not exists oauth_accounts (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    provider text not null,
    provider_id text not null,
    encrypted_refresh_token text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (provider, provider_id)
);

create index if not exists idx_oauth_accounts_user_id on oauth_accounts(user_id);

create table if not exists sessions (
    id text primary key,
    token_hash text unique not null,
    user_id uuid not null references users(id) on delete cascade,
    expires_at timestamptz not null,
    created_at timestamptz not null default now()
);

create index if not exists idx_sessions_token_hash on sessions(token_hash);
create index if not exists idx_sessions_user_id on sessions(user_id);
create index if not exists idx_sessions_expires_at on sessions(expires_at);

create table if not exists auth_code (
    id text primary key,
    token_hash text not null,
    encrypted_session_token text not null,
    user_id uuid not null references users(id) on delete cascade,
    expires_at timestamptz not null,
    created_at timestamptz not null default now()
);

create index if not exists idx_auth_code_expires_at on auth_code(expires_at);

create table if not exists provider_notifications_seen (
    jti text primary key,
    provider text not null,
    received_at timestamptz not null default now()
);

create index if not exists idx_provider_notifications_received_at on provider_notifications_seen(received_at);

-- ============================================================
-- Watchlists & Watch Tracking
-- ============================================================

create table if not exists watchlist_item (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    media_type text not null
        check (media_type in ('movie', 'series')),
    media_id integer not null,
    media_title text not null,
    poster_path text,
    status text not null default 'want_to_watch'
        check (status in ('want_to_watch', 'watched')),
    started_at timestamptz,
    finished_at timestamptz,
    last_watched_at timestamptz,
    watch_count integer not null default 0,
    genres text[] not null default '{}',
    runtime_minutes integer,
    vote_average real,
    release_year integer,
    metadata_refreshed_at timestamptz not null default now(),
    added_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (user_id, media_type, media_id)
);

create index if not exists idx_watchlist_user_status on watchlist_item(user_id, status);

create table if not exists watch_event (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    media_type text not null
        check (media_type in ('movie', 'episode', 'series', 'season')),
    media_id integer not null,
    media_title text not null,
    series_id integer,
    series_title text,
    season_number integer,
    episode_number integer,
    episode_count integer,
    season_count integer,
    watched_at timestamptz,
    timezone text not null default 'UTC',
    rewatch_number integer not null default 1,
    runtime_minutes integer,
    genres text[] not null default '{}',
    created_at timestamptz not null default now()
);

create index if not exists idx_watch_event_user_id on watch_event(user_id);

-- ============================================================
-- Gamification & Achievements
-- ============================================================

create table if not exists user_achievement (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    achievement_id text not null,
    earned_via_media_type text check (earned_via_media_type in ('movie', 'series')),
    earned_via_media_id integer,
    earned_via_media_title text,
    seen_at timestamptz,
    earned_at timestamptz not null default now(),
    unique (user_id, achievement_id)
);

create index if not exists idx_user_achievement_user_id on user_achievement(user_id);
