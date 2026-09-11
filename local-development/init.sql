-- ============================================================
-- Postgres Init Script
-- ============================================================

-- ============================================================
-- Extensions & Text Search
-- ============================================================

create extension if not exists pg_trgm;
create extension if not exists unaccent;

-- create pgvector extension
do $$ begin
    if exists (select 1 from pg_available_extensions where name = 'vector') then
        create extension if not exists vector;
    end if;
end $$;

-- unaccent() is STABLE and cannot appear in a generated column, but a text search
-- configuration that maps through unaccent can, because to_tsvector(regconfig, text) is immutable.
do $$ begin
    if not exists (select 1 from pg_ts_config where cfgname = 'simple_unaccent') then
        create text search configuration simple_unaccent (copy = simple);
        alter text search configuration simple_unaccent
            alter mapping for asciiword, asciihword, hword_asciipart, word, hword, hword_part
            with unaccent, simple;
    end if;
end $$;

-- array_to_string is declared STABLE, so it cannot appear in a generated column either.
create or replace function immutable_array_to_string(text[], text)
returns text language sql immutable parallel safe
as $$ select array_to_string($1, $2) $$;

-- unaccent() itself is STABLE; wrapping it makes the same promise the text search configuration makes.
create or replace function immutable_unaccent(text)
returns text language sql immutable parallel safe
as $$ select unaccent($1) $$;

-- slugify lowercases, strips accents, and collapses everything else to single dashes: 'Amélie' -> 'amelie'.
create or replace function slugify(text)
returns text language sql immutable parallel safe
as $$ select trim(both '-' from regexp_replace(lower(immutable_unaccent(coalesce($1, ''))), '[^a-z0-9]+', '-', 'g')) $$;

-- ============================================================
-- Catalog Entity Tables
-- ============================================================

/**
 * Movies
 */
create table if not exists movies (
    id                integer generated always as identity primary key,
    source_id         integer not null unique,
    slug              text generated always as (rtrim(id::text || '-' || slugify(title), '-')) stored,
    title             text not null default '',
    original_title    text,
    overview          text,
    release_date      date,
    runtime_minutes   integer,
    popularity        real not null default 0,
    vote_average      real,
    poster_path       text,
    genres            text[] not null default '{}',
    collection_id     integer,                                  -- collections.id
    payload           jsonb,
    search_vector     tsvector generated always as (
        setweight(to_tsvector('simple_unaccent', title), 'A') ||
        setweight(to_tsvector('simple_unaccent', coalesce(original_title, '')), 'B') ||
        setweight(to_tsvector('simple_unaccent', coalesce(overview, '')), 'C')
    ) stored,
    stale             boolean not null default false,
    fetched_at        timestamptz,                          -- hydration only; null on skeleton rows
    created_at        timestamptz not null default now(),
    updated_at        timestamptz not null default now()
);

create index if not exists idx_movies_search on movies using gin (search_vector);
create index if not exists idx_movies_title_trgm on movies using gin (title gin_trgm_ops);
create index if not exists idx_movies_stale on movies (fetched_at) where stale;
create index if not exists idx_movies_fetched_at on movies (fetched_at) where payload is not null;
create index if not exists idx_movies_needs_work on movies (popularity desc) where payload is null;

/**
 * TV Series
 */
create table if not exists series (
    id                    integer generated always as identity primary key,
    source_id             integer not null unique,
    slug                  text generated always as (rtrim(id::text || '-' || slugify(name), '-')) stored,
    name                  text not null default '',
    original_name         text,
    overview              text,
    first_air_date        date,
    next_air_date         date,
    in_production         boolean,
    total_seasons         integer,
    total_episodes        integer,
    episode_run_time      integer,
    popularity            real not null default 0,
    vote_average          real,
    poster_path           text,
    genres                text[] not null default '{}',
    payload               jsonb,
    search_vector         tsvector generated always as (
        setweight(to_tsvector('simple_unaccent', name), 'A') ||
        setweight(to_tsvector('simple_unaccent', coalesce(original_name, '')), 'B') ||
        setweight(to_tsvector('simple_unaccent', coalesce(overview, '')), 'C')
    ) stored,
    stale                 boolean not null default false,
    fetched_at            timestamptz,                          -- hydration only; null on skeleton rows
    created_at            timestamptz not null default now(),
    updated_at            timestamptz not null default now()
);

create index if not exists idx_series_search on series using gin (search_vector);
create index if not exists idx_series_name_trgm on series using gin (name gin_trgm_ops);
create index if not exists idx_series_stale on series (fetched_at) where stale;
create index if not exists idx_series_fetched_at on series (fetched_at) where payload is not null;
create index if not exists idx_series_needs_work on series (popularity desc) where payload is null;

/**
 * TV Series Seasons. Skeleton rows come from the series document; the payload arrives when the
 * season itself is fetched.
 */
create table if not exists seasons (
    id             integer generated always as identity primary key,
    series_id      integer not null,                             -- series.id
    season_number  integer not null,
    source_id      integer not null unique,
    name           text not null default '',
    payload        jsonb,
    stale          boolean not null default false,
    fetched_at     timestamptz,                          -- hydration only; null on skeleton rows
    created_at     timestamptz not null default now(),
    updated_at     timestamptz not null default now(),
    -- deferrable so the check runs at the end of the statement: TMDB renumbers seasons, and a bulk
    -- upsert moves several rows through each other's numbers before the set is consistent again
    unique (series_id, season_number) deferrable initially immediate
);

create index if not exists idx_seasons_fetched_at on seasons (fetched_at);
create index if not exists idx_seasons_stale on seasons (series_id) where stale;

/**
 * TV Series Episodes. Skeleton rows come from the season document; the payload arrives when the
 * episode itself is fetched.
 */
create table if not exists episodes (
    id              integer generated always as identity primary key,
    series_id       integer not null,                            -- series.id
    season_number   integer not null,
    episode_number  integer not null,
    source_id       integer not null unique,
    name            text not null default '',
    payload         jsonb,
    stale           boolean not null default false,
    fetched_at      timestamptz,                          -- hydration only; null on skeleton rows
    created_at      timestamptz not null default now(),
    updated_at      timestamptz not null default now(),
    unique (series_id, season_number, episode_number) deferrable initially immediate
);

create index if not exists idx_episodes_fetched_at on episodes (fetched_at);
create index if not exists idx_episodes_stale on episodes (series_id) where stale;

/**
 * Movie Collections
 */
create table if not exists collections (
    id         integer generated always as identity primary key,
    source_id  integer not null unique,
    slug       text generated always as (rtrim(id::text || '-' || slugify(name), '-')) stored,
    name       text not null default '',
    payload    jsonb not null,
    fetched_at timestamptz not null default now(),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

/**
 * People
 */
create table if not exists people (
    id                   integer generated always as identity primary key,
    source_id            integer not null unique,
    slug                 text generated always as (rtrim(id::text || '-' || slugify(name), '-')) stored,
    name                 text not null default '',
    date_of_birth        date,
    date_of_death        date,
    profile_path         text,
    known_for_department text,
    also_known_as        text[] not null default '{}',
    popularity           real not null default 0,
    payload              jsonb,
    insights             jsonb,     -- augur result
    insights_at          timestamptz,
    search_vector        tsvector generated always as (
        setweight(to_tsvector('simple_unaccent', name), 'A') ||
        setweight(to_tsvector('simple_unaccent', immutable_array_to_string(also_known_as, ' ')), 'B')
    ) stored,
    stale                boolean not null default false,
    fetched_at           timestamptz,                          -- hydration only; null on skeleton rows
    created_at           timestamptz not null default now(),
    updated_at           timestamptz not null default now()
);

create index if not exists idx_people_search on people using gin (search_vector);
create index if not exists idx_people_name_trgm on people using gin (name gin_trgm_ops);
create index if not exists idx_people_stale on people (fetched_at) where stale;
create index if not exists idx_people_fetched_at on people (fetched_at) where payload is not null;
create index if not exists idx_people_needs_work on people (popularity desc) where payload is null;

-- ============================================================
-- Jobs
-- ============================================================

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
    media_type text not null check (media_type in ('movie', 'series')),
    media_id integer not null,
    media_title text not null,
    status text not null default 'want_to_watch' check (status in ('want_to_watch', 'in_progress', 'watched')),
    started_at timestamptz,
    finished_at timestamptz,
    last_watched_at timestamptz,
    watch_count integer not null default 0,
    episodes_watched integer not null default 0,
    seasons_watched integer not null default 0,
    added_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (user_id, media_type, media_id)
);

create index if not exists idx_watchlist_user_status on watchlist_item(user_id, status);

create table if not exists watch_event (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    media_type text not null check (media_type in ('movie', 'episode', 'series', 'season')),
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
    created_at timestamptz not null default now(),
    unique (user_id, media_type, media_id)
);

create index if not exists idx_watch_event_user_id on watch_event(user_id);
create index if not exists idx_watch_event_user_series on watch_event(user_id, series_id)
    where series_id is not null;

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
