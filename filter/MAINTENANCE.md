# Maintaining the CEL Filter Engine

Use the memo and attachment schemas in `schema.go` to expose a field to CEL.
Define its SQLite column, type, supported operators, and optional JSON path.
Declare the field in `EnvOptions`. Use `FieldKindJSONBool` for facts derived
from note content and `FieldKindExists` for facts about related rows. Attachment
presence, for example, follows the current relationships in SQLite.

Validate field types and operators in `parser.go`. Render the resulting IR in
`render.go`, using bound arguments for user-provided values. `Compile` checks
the CEL expression and returns a `Program`. `Program.Render` produces a SQL
fragment with its arguments. `CompileToStatement` combines the two steps.

Keep the `?` placeholders and argument order aligned when composing SQL for the
store. A branch folded to a constant contributes no arguments. Reuse
`DefaultEngine` and `DefaultAttachmentEngine`, which cache their respective CEL
environments.

Extend the relevant renderer assertions when changing a predicate. Verify
SQLite execution when the change affects stored rows, ordering, matching,
or omitted JSON values. Run `go test ./filter/... ./store/... ./server/mcp/...`
for the filter package and its main integration paths.
