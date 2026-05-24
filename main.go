package main

import (
	"embed"

	"allstar-yaamon/cmd/yaamon"
)

//go:embed web
var webFS embed.FS

func main() {
	yaamon.Execute(webFS)
}
