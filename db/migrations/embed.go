package migrations

import "embed"

// FS contains versioned PostgreSQL migrations bundled with the platform binary.
//
//go:embed *.sql
var FS embed.FS
