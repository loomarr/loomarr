Every table and column Loomarr keeps, in plain words, grouped by the part of the app that uses
it. It's for anyone looking inside the database: to understand what Loomarr remembers, answer a
question the app doesn't show, or report a problem precisely.

## Looking without breaking anything

Look at a **copy**, never the live file. `make db-view` opens a copy of this database in a
browser-based viewer and deletes the copy when you stop it (see
[the dev loop](../contributing/dev-loop.md)). For a quick question about a SQLite install, a
read-only shell on a copy or a nightly backup works too:

```bash
sqlite3 -readonly loomarr-copy.db '.tables'
```

Don't change rows by hand. Loomarr checks what it writes, and a hand edit skips those checks.
Change things in the app, or open an issue if the app can't do what you need.

On Postgres, these descriptions are also stored as table and column comments, so most SQL
clients show them beside the data.

## Which table answers "what titles do we have"

`library_items`. A title is in the library when one of its `library_files` rows is present
(`missing_at_ms` is 0). The other title tables are layers around it, each with one job:

1. **`inventory_*`: what each importer reported.** One item per movie, show or episode, however
   many importers (a folder scan, an Emby or Jellyfin server) report it, with the files that play
   it. An item's id is fixed by its first report, so a title that comes back gets the same id.
2. **`library_*`: the answer.** One `library_items` row per title, with the same id as its
   inventory item, written in the same transaction as each inventory write, plus its details,
   files, artwork and collections. When the sources disagree, this is the one answer the app shows.
3. **`titles`: what channels asked for.** A movie or show a channel wants, tracked from wanted to
   available through Seerr or the arr apps. Once it arrives, `titles.library_id` says which item it
   became. Until the switch to Loomarr's own scanner (#2151) that is the media server's id, which
   the switch rewrites to the library's.

Every layer is pruned: the nightly housekeeping deletes reports, files and library rows long gone
(each table's retention says when).

## Which tables are growing

Every night, after its clean-up, the housekeeping task records how many rows each table holds in
`database_table_rows`, and keeps 400 days of counts. A table that keeps growing month after month
is one nothing trims. This lists the ten that grew most over the last 30 days, on SQLite or
Postgres:

```sql
SELECT latest.table_name, latest.row_count AS rows_now,
       latest.row_count - COALESCE(earlier.row_count, 0) AS grew_by
FROM database_table_rows latest
LEFT JOIN database_table_rows earlier
  ON earlier.table_name = latest.table_name
 AND earlier.recorded_at = latest.recorded_at - 30 * 86400
WHERE latest.recorded_at = (SELECT MAX(recorded_at) FROM database_table_rows)
ORDER BY grew_by DESC
LIMIT 10;
```

If Prometheus scrapes Loomarr, the latest counts are also the `loomarr_database_table_rows` gauge,
one series per table, so `delta(loomarr_database_table_rows[30d])` graphs the same growth.

## Reading the values

- **Times** are whole numbers counting from 1 January 1970 UTC. The name says the unit: a column
  ending in `_ms` counts milliseconds, `_ns` nanoseconds, and any other counts seconds. In SQLite,
  `datetime(col, 'unixepoch')` turns seconds into a date and `datetime(col / 1000, 'unixepoch')`
  does the same for milliseconds.
- **Yes/no** columns hold `1` for yes and `0` for no.
- Columns ending in **`_json`** hold a JSON document; their description says what's inside.
- **May be empty** in the Type column means the column can be `NULL`. A **key** column
  identifies the row. An arrow (→) names the table and column a value points at.
- Types are shown as SQLite declares them. Postgres uses its own equivalent for each.
