# CEL Filter Engine

The filter engine converts CEL expressions for memo and attachment queries into
SQLite predicates with bound arguments.

1. **Parsing** – CEL expressions are parsed with `cel-go` and validated against
   the memo or attachment environment declared in `schema.go`.
2. **Normalization** – the raw CEL AST is converted into an intermediate
   representation (IR) defined in `ir.go`. It represents logical operators,
   comparisons, list membership, and other conditions. This
   step enforces schema rules (e.g. operator compatibility, type checks).
3. **Rendering** – the renderer in `render.go` walks the IR and produces a SQL
   fragment plus bound arguments in placeholder order.

Use `filter.DefaultEngine()` for memos and `filter.DefaultAttachmentEngine()`
for attachments. Each caches the corresponding CEL environment.

```go
engine, _ := filter.DefaultEngine()
stmt, _ := engine.CompileToStatement(ctx, `has_task_list && visibility == "PUBLIC"`)
// stmt.SQL  -> "(JSON_EXTRACT(`memo`.`payload`, '$.property.hasTaskList') IS TRUE AND `memo`.`visibility` = ?)"
// stmt.Args -> ["PUBLIC"]
```

## Core Files

| File          | Responsibility                                                                  |
| ------------- | ------------------------------------------------------------------------------- |
| `schema.go`   | Declares memo and attachment fields, types, backing columns, and CEL options   |
| `ir.go`       | IR node definitions used across the pipeline                                    |
| `parser.go`   | Converts CEL `Expr` into IR while applying schema validation                    |
| `render.go`   | Translates IR into SQLite predicates and bound arguments                         |
| `engine.go`   | Glue between the phases; exposes `Compile`, `CompileToStatement`, and `DefaultEngine` |
| `helpers.go`  | Convenience helpers for store integration (appending conditions)                |

## SQL Generation Notes

- **Placeholders** — Each `?` consumes one argument. Append the fragment and
  its arguments in the order they appear in the full query.
- **JSON Fields** — Memo metadata lives in `memo.payload`. The renderer handles
  JSON paths with `JSON_EXTRACT` and list elements with `json_each`.
- **Time Fields** — `created_ts`, `updated_ts`, and attachment `create_time` are
  CEL `timestamp` values. Express instants with the `now` variable,
  `duration("…")` (e.g. `created_ts >= now - duration("24h")`), or
  `timestamp("2006-01-02T15:04:05Z")` / `timestamp(<epoch-seconds>)`. These fold
  to epoch seconds at compile time — `now` is frozen once per compile (injectable
  for tests via the engine clock) — so the backing columns stay unchanged.
- **Tag Operations** — `tag in [...]`, `"tag" in tags`, and tag comprehension
  predicates evaluate individual JSON array elements. Equality and string
  matching are case-sensitive and use the original code points. `%` and `_` in
  string operands are literal. `TagBranchExpression` matches an exact path or
  a descendant separated by `/`, including older payloads that only store the
  literal child path.
- **Boolean Flags** — Fields such as `has_task_list` render as an `IS TRUE` check.
  An omitted key counts as false. `flag == false`, `flag != true`, and `!flag`
  match the same memos.
- **Presence Flags** — `has_location` checks legacy location metadata in
  `memo.payload` (`$.location`). The memo API preserves existing locations and
  ignores location fields on writes. A missing key and an explicit JSON null both
  count as absent. Any other value, including an empty object,
  counts as present. Only `==`/`!=` against a boolean literal (or bare/negated
  use) is allowed.
- **Related-Row Flags** — `has_attachments` renders as `EXISTS` over
  `attachment` rows whose `memo_id` is the memo. `has_image` ORs the payload
  flag `property.hasInlineImage` (Markdown image syntax in the content) with an
  `EXISTS` over attachments whose `type` starts with `image/`, ignoring case.
  These facts are read at query time because attachment binding changes without
  the content changing; never copy them into the payload. Comparisons follow the
  presence flags.
- **String Matching** — `content.contains(x)`, `content.startsWith(x)`, and
  `content.endsWith(x)` render as Unicode-folded `LIKE` with its
  metacharacters (`%`, `_`, `\`) escaped. Available on scalar string fields whose
  schema sets `SupportsContains` (memo `content`; attachment `filename`,
  `mime_type`).
- **Regex** — `field.matches("pattern")` uses the Go-backed `REGEXP` function
  registered in `store/db/sqlite/functions.go`. `cel.ValidateRegexLiterals()`
  validates patterns against Go's RE2 syntax before rendering.
- **Tag `all()` / `exists_one()`** — `tags.all(t, <pred>)` matches only non-empty
  tag sets where every element satisfies the predicate; `tags.exists_one(t,
  <pred>)` matches when exactly one element does (`COUNT(...) = 1`). Both iterate
  over `json_each`.
- **Timestamp Accessors** — `created_ts.getFullYear()`, `getMonth()`, `getDate()`,
  `getDayOfMonth()`, `getDayOfWeek()`, `getDayOfYear()`, `getHours()`,
  `getMinutes()`, `getSeconds()` use `strftime` in UTC. Results follow CEL's base (0-based
  month, 0-based day-of-week with 0 = Sunday). The same accessors on `now` fold
  to literal date parts of the frozen evaluation time (UTC), so saved filters
  like `created_ts.getMonth() == now.getMonth() && created_ts.getDate() ==
  now.getDate()` ("on this day") re-resolve on every compile. Accessors take
  no timezone argument.
- **Set Operations** — `sets.contains(tags, [...])`, `sets.intersects(tags, [...])`,
  and `sets.equivalent(tags, [...])` expand into exact-membership checks.
  `equivalent` also compares the number of tags.
- **`size()`** — `size(tags)` reads the JSON array length. On strings such as
  `content`, `LENGTH` returns the code-point count.
- **Arithmetic** — `+`, `-`, `*`, `/`, `%` constant-fold on literal/`now`/`duration`
  operands (division and modulo guard against a zero divisor).

## Typical Integration

1. Fetch the engine for memos or attachments.
2. Call `CompileToStatement` with the CEL expression.
3. Append the emitted SQL fragment/args to the existing `WHERE` clause.
4. Execute the resulting query through the store driver.

`filter.AppendConditions` compiles multiple filters and appends their SQL and
arguments to the existing query.
