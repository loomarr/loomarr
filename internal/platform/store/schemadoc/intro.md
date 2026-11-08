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

## Reading the values

- **Times** are whole numbers counting from 1 January 1970 UTC. Some columns count seconds and
  some count milliseconds; each one says which. In SQLite,
  `datetime(col, 'unixepoch')` turns seconds into a date and `datetime(col / 1000, 'unixepoch')`
  does the same for milliseconds.
- **Yes/no** columns hold `1` for yes and `0` for no.
- Columns ending in **`_json`** hold a JSON document; their description says what's inside.
- **May be empty** in the Type column means the column can be `NULL`. A **key** column
  identifies the row. An arrow (→) names the table and column a value points at.
- Types are shown as SQLite declares them. Postgres uses its own equivalent for each.
