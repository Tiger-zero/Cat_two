# Cat_two Protocol Buffers

## Prerequisites

- [Buf CLI](https://buf.build/docs/cli/installation/)

Run the commands below from `proto/`.

## Generate

```sh
buf generate
```

The generation config writes Go and OpenAPI files to `gen/` and TypeScript files to `../web/src/types/proto/`. [Generation config](buf.gen.yaml)

## Check

```sh
buf lint
buf format --diff --exit-code
```

## Deployment settings

Deployment JSON follows the `memos.api.Configuration` schema. Use `memos config validate` to check the files
and `memos config plan` to compare them with the SQLite instance. The server reports older standalone
configuration files in its startup logs. The supported directory is selected with `--config-dir` or
`MEMOS_CONFIG_DIR`. [Configuration schema](api/configuration.proto) · [Configuration commands](../cmd/memos/config.go) · [Startup](../cmd/memos/main.go)
