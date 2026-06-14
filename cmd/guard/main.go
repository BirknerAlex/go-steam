package main

import (
	"flag"
	"time"
	"github.com/BirknerAlex/go-steam/internal/totp"
)

func main() {
	var (
		secret = flag.String("secret", "", "Base64 TOTP shared secret for auto Steam Guard (from mobile authenticator)")
	)
	flag.Parse()

	if *secret == "" {
		flag.Usage()
		return
	}

	totpCode, err := totp.GenerateAuthCode(*secret, time.Now())
	if err != nil {
		panic(err)
	}

	println(totpCode)
}
