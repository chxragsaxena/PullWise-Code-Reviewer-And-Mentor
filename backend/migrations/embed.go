// Package migrations embeds the SQL schema so the server can apply it itself.
package migrations

import "embed"

// Files holds every NNNN_name.sql migration, applied in name order.
//
//go:embed *.sql
var Files embed.FS
