package main

import (
	zip "github.com/yeka/zip"

	_ "embed"
)

//go:embed targets/default_darwin.quack
var defaultQuack []byte

func DefaulRootPaths() []string {
	return []string{
		"/",
	}
}

func GetPathsRaw(cfg Configuration) ([]CollectTarget, error) {
	return GetPaths(cfg)
}

func CollectFileRaw(cfg Configuration, archive *zip.Writer, path string) (string, int64, string, string, error) {
	return CollectFile(cfg, archive, path)
}
