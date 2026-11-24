# Store tests

Run from the repository root. The store suite and Backend Tests use isolated SQLite databases. The suite includes fresh
install, incremental migration and CalVer upgrade fixtures.

```bash
go test -v ./store/...
```

## Upgrade coverage

```bash
go test -v -count=1 -timeout 30m -run 'TestMigration|TestUpgrade|TestFreshInstall' ./store/test/...
```

The routine `TestUpgradeFromCalendarRelease` uses local store fixtures for month changes, point releases, RC promotion and year
changes. It checks data preservation, post-upgrade writes and restart behavior. The application version and database schema version
advance independently.

To verify a container upgrade with an isolated volume, pass the previous Cat_two image to the
[container smoke script](../../scripts/release_smoke_test.sh) with `--previous-image`.
