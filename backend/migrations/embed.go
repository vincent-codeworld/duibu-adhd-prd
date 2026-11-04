package migrations

import "embed"

// FS holds SQL migration files next to this package (source of truth).
//
//go:embed *.sql
var FS embed.FS
