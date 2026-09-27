# Back up and restore

**For:** household admins who want to get Loomarr back after a lost disk or a bad upgrade.
**You'll get:** automatic backups you've checked, the one file they leave out, and the steps to
restore.

## What a backup holds

On SQLite, the default database, Loomarr writes a database backup every night. It holds your
settings, channels, people and their access, and your saved credentials, encrypted. On Postgres,
back up with `pg_dump` instead, as [Upgrade](upgrade.md#back-up) shows.

It leaves out:

- **The installation key** (`/data/encryption.key`). The credentials in a backup are encrypted
  with it, and it's left out on purpose so a copied backup is useless by itself. Keep a copy
  somewhere safe and separate. Without it, a restored database can't read its saved credentials.
- **Files on disk:** filler, cached artwork and images you uploaded. Back up the `/data` volume
  for those.

## Check the automatic backups

Go to **Settings → System → Backup**. By default Loomarr writes a backup at 03:30 every night,
keeps the last 7, and writes them to `/data/backups`.

That folder is on the same disk as the database, so a failed disk takes both. Set **Backup
location** to another disk or a network share you've mounted into the container. Every backup
setting is listed in the
[settings reference](https://mantonx.github.io/loomarr/reference/settings/#backup).

## Take one now

Before an upgrade or a big change, download a fresh backup from **Settings → System → Backup**.
To script it, or if you run Postgres, follow the steps in [Upgrade](upgrade.md#back-up).

## Restore

1. **Stop Loomarr.** A clean stop writes everything into the database file.
2. **Put the installation key back** at `/data/encryption.key`, if it's missing or you're
   restoring onto a new host. It must be the key from the same install as the backup.
3. **Replace the database.** On SQLite, move the current `/data/loomarr.db` aside and copy the
   backup in its place, named `loomarr.db`. If `loomarr.db-wal` or `loomarr.db-shm` is still
   there, move it aside too. Give the file to uid and gid 65532, the user Loomarr runs as. On
   Postgres, follow [Rolling back](upgrade.md#rolling-back), which replaces only the Loomarr
   database.
4. **Start Loomarr** and wait for `/v1/readyz` to answer.
5. **Check it:** sign in, open a channel and play it. Keep the old database until you're sure.

If Loomarr refuses to start because the installation key doesn't match the database, the key is
from a different install. See [Troubleshooting](troubleshooting.md#database-secret-encryption).
