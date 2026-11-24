// Package store is the persistence layer: the Store facade, the Driver
// interface implemented by store/db/sqlite, migrations, seed
// data, and the in-memory cache.
//
// Writes that must validate what they read run in runSerializable, which owns
// the transaction and its retries. The SQLite methods take the shared *sql.Tx.
//
// Layering: store may import markdown, filter, proto/gen, and
// internal. It must not import core or server; business rules that need
// several store calls live in core.
package store
