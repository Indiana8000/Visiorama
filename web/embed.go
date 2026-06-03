package web

import "embed"

//go:embed app/dist
var DistFS embed.FS
