package main

import (
	_ "embed"
)

//go:embed targets/default_darwin.quack
var defaultQuack []byte
