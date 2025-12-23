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
