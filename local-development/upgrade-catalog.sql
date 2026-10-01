-- ============================================================
-- Production cutover helpers, run by `make catalog-restore` around pg_restore.
-- Invoke with -v step=extensions (before the restore) or -v step=finish (after it).
-- Every statement is idempotent and guarded, so the file is safe on a fresh database too.
-- ============================================================

\if :{?step}
\else
  \echo 'usage: psql -v step=extensions|finish -f upgrade-catalog.sql'
  \quit 2
\endif

select :'step' = 'extensions' as is_extensions \gset
select :'step' = 'finish' as is_finish \gset

\if :is_extensions
create extension if not exists pg_trgm;
create extension if not exists unaccent;

do $$ begin
    if exists (select 1 from pg_available_extensions where name = 'vector') then
        create extension if not exists vector;
    end if;
end $$;

do $$ begin
    if not exists (select 1 from pg_ts_config where cfgname = 'simple_unaccent') then
        create text search configuration simple_unaccent (copy = simple);
        alter text search configuration simple_unaccent
            alter mapping for asciiword, asciihword, hword_asciipart, word, hword, hword_part
            with unaccent, simple;
    end if;
end $$;

create or replace function immutable_array_to_string(text[], text)
returns text language sql immutable parallel safe
as $$ select array_to_string($1, $2) $$;
\endif

\if :is_finish
-- Carry forward what costs money or breaks a watchlist: augur insights and series totals.
-- The old tables speak the provider's ids, so join on source_id.
do $$ begin
    if to_regclass('interesting_info') is not null then
        update people p set insights = i.data, insights_at = i.fetched_at
        from interesting_info i
        where i.entity_type = 'person' and i.entity_id = p.source_id;
    end if;

    if to_regclass('series_metadata') is not null then
        update series s set
            total_episodes = coalesce(s.total_episodes, m.total_episodes),
            total_seasons  = coalesce(s.total_seasons,  m.total_seasons),
            in_production  = coalesce(s.in_production,  m.in_production),
            next_air_date  = coalesce(s.next_air_date,  m.next_air_date)
        from series_metadata m
        where m.series_id = s.source_id;
    end if;
end $$;

-- User rows hold the provider's ids, and a catalog row's id is its source_id, so they already
-- point at the right rows and nothing is rewritten. What is listed here is any user row whose title
-- the catalog does not hold, so it can be looked at; it keeps rendering from its stored title.
select 'watchlist_item' as tbl, w.media_type, w.media_id as source_id
from watchlist_item w
left join movies m on w.media_type = 'movie'  and m.source_id = w.media_id
left join series s on w.media_type = 'series' and s.source_id = w.media_id
where m.id is null and s.id is null
union all
select 'watch_event', e.media_type, e.media_id
from watch_event e
left join movies m   on e.media_type = 'movie'   and m.source_id  = e.media_id
left join series s   on e.media_type = 'series'  and s.source_id  = e.media_id
left join seasons se on e.media_type = 'season'  and se.source_id = e.media_id
left join episodes ep on e.media_type = 'episode' and ep.source_id = e.media_id
where m.id is null and s.id is null and se.id is null and ep.id is null;


-- What the new binary no longer reads.
alter table watchlist_item
    drop column if exists poster_path, drop column if exists genres, drop column if exists runtime_minutes,
    drop column if exists vote_average, drop column if exists release_year, drop column if exists metadata_refreshed_at;
drop table if exists series_metadata;
drop table if exists season_cast_cache;
drop table if exists interesting_info;
\endif
