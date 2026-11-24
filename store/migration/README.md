# SQLite migration baseline

The minimum supported database schema is **0.31.8**, shipped in **v0.31.0**.
This is a fixed baseline, independent of application release numbering.

- Fresh installations use [`sqlite/LATEST.sql`](sqlite/LATEST.sql) and record the current schema version.
- Existing databases below the baseline must first run
  [v0.31.0](https://github.com/usememos/memos/releases/tag/v0.31.0) successfully.
- Databases older than v0.22, or with no schema version recorded, must first run
  [v0.25.3](https://github.com/usememos/memos/releases/tag/v0.25.3), then v0.31.0.
- Invalid schema versions and databases newer than the binary supports are rejected.

Historical SemVer migration scripts are retained in Git history and released
binaries. Update `sqlite/LATEST.sql` for fresh installations and add an
incremental migration for each schema change. Keep databases at schema 0.31.8
upgradeable.

New migrations use `YY.MM/NN__description.sql`, where `NN` starts at `00`.
Add the SQL file under `sqlite/`. Each sequence number is unique within a month.
For example, `26.09/00__add_column.sql` records schema `26.9.1`, and the next
migration records `26.9.2`. The year, month and sequence are compared numerically.
A release without schema changes keeps the existing schema version. Calendar schemas
sort after the legacy 0.x baseline. Migration directories keep months zero-padded
(`01` through `12`), while stored schema markers omit the padding. This lets older binaries' SemVer comparisons
recognize a calendar schema as newer and reject a downgrade.

## Coordinating an upgrade

Back up the database and stop Cat_two and any other database writers. Start the
new version to apply the migrations.

Migrations run at startup before the HTTP server starts. Index builds can take
time on large attachment tables and block concurrent writes. SQLite commits the
schema changes and version marker together. If a migration fails, both roll
back. On restart, Cat_two skips migrations whose versions are already recorded.
