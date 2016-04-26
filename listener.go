package moria

import (
	"log"
	"net/http"
	"os"
)

// Listen starts the api gateway
func Listen(e *Exchange) {
	// Listen for HTTP requests from API clients and forward them to the
	// appropriate service backend.
	port := os.Getenv("PORT")
	log.Printf("Listening for HTTP requests on port %v", port)
	err := http.ListenAndServe("localhost:"+port, Log(e.mux))
	if err != nil {
		log.Print(err)
	}
}

// Log logs api gateway requests
func Log(handler http.Handler) http.Handler {
	wrapper := func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("%s %s %s", request.RemoteAddr, request.Method, request.URL.Path)
		handler.ServeHTTP(writer, request)
	}
	return http.HandlerFunc(wrapper)
}
