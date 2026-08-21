package web

import "embed"

// Assets embeds all web templates and static files into the binary.
//
//go:embed templates/* static/*
var Assets embed.FS
