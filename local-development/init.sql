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
