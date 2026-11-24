# Cat_two Protocol Buffers

## Prerequisites

- [Buf CLI](https://buf.build/docs/cli/installation/)

Run the commands below from `proto/`.

## Generate

```sh
buf generate
```

Generation writes Go messages and service bindings to `gen/`, and TypeScript bindings to `../web/src/types/proto/`. The web app uses Connect under `/memos.api.*`. HTTP annotations supply the embedded `gen/openapi.yaml` for MCP tool schemas. [Generation config](buf.gen.yaml) · [MCP schemas](../server/mcp/README.md#request-flow)

## Check

```sh
buf lint
buf format --diff --exit-code
```

## Deployment settings

Deployment JSON follows the `memos.api.Configuration` schema for general settings and the single administrator. Use `memos config validate` to check the files
and `memos config plan` to compare them with the SQLite instance. The server reports older standalone
configuration files in its startup logs. The supported directory is selected with `--config-dir` or
`MEMOS_CONFIG_DIR`. [Configuration schema](api/configuration.proto) · [Configuration commands](../cmd/memos/config.go) · [Startup](../cmd/memos/main.go)
