package offhand

import (
	"log"
	"os"
)

var (
	TestLogger Logger = log.New(os.Stderr, "offhand/", log.Lshortfile)
)

func init() {
	log.SetFlags(0)
}
