// Command dwfake serves the in-memory DocuWare used by the tests, so dw can be
// tried without a DocuWare server:
//
//	go run ./internal/dwfake/cmd/dwfake
//	dw login --url http://127.0.0.1:<port> --user peggy   (password: s3cret)
package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/rechedev9/docuware-cli/internal/dwfake"
)

func main() {
	s := dwfake.New()
	defer s.Close()
	fmt.Printf("fake DocuWare at %s (user %s / %s, client %s / %s)\n", s.URL, s.Username, s.Password, s.ClientID, s.ClientSecret)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}
