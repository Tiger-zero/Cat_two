# Store tests

Run from the repository root. The store suite uses isolated SQLite databases. Changes to the schema or database opening logic should cover fresh initialization, reopening a current database, upgrading the preceding single-account database, and safe rejection of incompatible versions.

```bash
go test -v ./store/...
```

The [container smoke script](../../scripts/release_smoke_test.sh) checks first boot and persistence after container recreation with a dedicated data volume.
