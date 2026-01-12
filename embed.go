package main

import (
	"embed"
	"io/fs"
)

//go:embed frontend/*
var frontendFiles embed.FS

// GetFrontendFS returns the embedded frontend filesystem
func GetFrontendFS() (fs.FS, error) {
	return fs.Sub(frontendFiles, "frontend")
}
