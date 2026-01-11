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
