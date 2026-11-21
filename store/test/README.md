# Store tests

Run from the repository root. The default store suite and Backend Tests use isolated SQLite databases. The suite includes fresh
install, incremental migration and CalVer upgrade fixtures.

```bash
go test -v ./store/...
```

## Optional compatibility tests

Set `DRIVER` explicitly to run a compatibility suite with Docker.

```bash
DRIVER=mysql go test -v ./store/test/... # MySQL 8.4
DRIVER=mysql MYSQL_VARIANT=mariadb go test -v ./store/test/... # MariaDB 11.4.13
DRIVER=postgres go test -v ./store/test/...
```

MariaDB uses the application's `mysql` driver, dialect, and migrations. The `MYSQL_VARIANT` selector accepts `mysql` (the default)
and `mariadb`. The MariaDB container enables `ONLY_FULL_GROUP_BY` alongside its strict SQL mode.

MariaDB 11.4.13 is the tested compatibility target. Deployments need InnoDB and utf8mb4. Other MariaDB versions, database conversion
between MySQL and MariaDB, and replication between them are outside this coverage. Text ordering and regex behavior can differ;
timestamp accessors use the database session timezone. Exact tag and content-guard comparisons retain explicit binary semantics.

## Upgrade coverage

```bash
DRIVER=sqlite go test -v -count=1 -timeout 30m -run 'TestMigration|TestUpgrade|TestFreshInstall' ./store/test/...
```

The routine `TestUpgradeFromCalendarRelease` uses local store fixtures for month changes, point releases, RC promotion and year
changes. It checks data preservation, post-upgrade writes and restart behavior. The application version and database schema version
advance independently.

To verify a container upgrade with an isolated volume, pass the previous Cat_two image to the
[container smoke script](../../scripts/release_smoke_test.sh) with `--previous-image`.
