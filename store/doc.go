// Package store provides the persistence facade, SQLite schema, driver
// interface, and in-memory cache.
//
// Writes that must validate what they read run in runSerializable, which owns
// the transaction and its retries. The SQLite methods take the shared *sql.Tx.
//
// Layering: store may import markdown, filter, proto/gen, and
// internal. It must not import core or server; business rules that need
// several store calls live in core.
package store
