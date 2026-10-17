// Browser-test fixture: real API routes with the production default of no gate.
package main

import (
	"log"
	"net/http"

	"github.com/openexec/openexec/pkg/api"
)

func main() {
	mux := http.NewServeMux()
	server := &api.Server{}
	server.RegisterRoutes(mux)
	log.Fatal(http.ListenAndServe("127.0.0.1:18080", mux))
}
