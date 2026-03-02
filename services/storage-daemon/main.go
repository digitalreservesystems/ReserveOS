package main

import (
	"log"
	"os"

	"reserveos/services/storage-daemon/storaged"
)

func main() {
	cfgPath := "config/services/storage-daemon/service.json"
	if len(os.Args) > 1 { cfgPath = os.Args[1] }

	s, err := storaged.New(cfgPath)
	if err != nil { log.Fatal(err) }
	log.Fatal(s.Run())
}
