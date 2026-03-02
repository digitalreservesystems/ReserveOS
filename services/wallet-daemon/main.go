package main

import (
	"log"
	"os"

	"reserveos/services/wallet-daemon/walletd"
)

func main() {
	cfgPath := "config/services/wallet-daemon/service.json"
	if len(os.Args) > 1 { cfgPath = os.Args[1] }

	s, err := walletd.New(cfgPath)
	if err != nil { log.Fatal(err) }
	log.Fatal(s.Run())
}
