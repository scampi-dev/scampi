package doit

import "embed"

//go:embed cue/**
var EmbeddedSchemaModule embed.FS
