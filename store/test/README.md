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

## Optional legacy Memos upgrades

Run [Upgrade Smoke (legacy Memos)](../../.github/workflows/upgrade-smoke.yml) manually in GitHub Actions, or run this command with Docker.

```bash
LEGACY_MEMOS_UPGRADE_TESTS=1 DRIVER=sqlite go test -v -count=1 -timeout 30m \
  -run '^TestUpgrade(FromBaselineRelease|ThroughBaselineRelease)$' ./store/test/...
```

The tests run released Memos binaries against temporary databases. The optional driver selectors above also apply to local runs.

- Before `v0.31.0`: `TestUpgradeThroughBaselineRelease` initializes a database with released `0.30.0`, verifies the current
  migrator refuses a direct upgrade without modifying data or settings, then upgrades through released `0.31.0` to current.
- From `v0.31.0`: `TestUpgradeFromBaselineRelease` initializes a database with released `0.31.0`, then checks preservation of
  the account, memo, saved View, and access mode, post-upgrade writes, and reopening the database after upgrade.
