package relink

import (
	"log"
	"os"
)

var (
	TestLogger Logger = log.New(os.Stderr, "relink/", log.Lshortfile)
)

func init() {
	log.SetFlags(0)
}
