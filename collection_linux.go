package main

import (
	_ "embed"
)

//go:embed targets/default_linux.quack
var defaultQuack []byte
