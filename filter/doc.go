// Package filter compiles the CEL filter expressions accepted by the list
// endpoints into an intermediate representation and renders it into SQLite
// SQL. The schemas define filterable memo and attachment fields.
//
// Layering: filter imports only third-party packages and internal. It must not
// import store, core, or server. The SQLite driver imports filter.
package filter
