package moria

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
)

type conflict struct {
	ExistingAddresses []string `json:"existing_addresses"`
	AttemptedConflict string   `json:"AttemptedConflict"`
}

// PatternHandler keeps track of backend service addresses that are registered to
// handle a URL pattern.
type PatternHandler struct {
	Pattern   string
	Addresses []string `json:"addresses"`
}

// Mux is an HTTP request multiplexer.  It matches the URL of
// each incoming request against a list of registered patterns to find the
// service that can respond to it and proxies the request to the appropriate
// backend.  Pattern matching logic is based on pat.go.
// ** FROM https://github.com/jkakar/switchboard
type Mux struct {
	rw     sync.RWMutex                 // Synchronize access to routes map.
	routes map[string][]*PatternHandler // Patterns mapped to backend services.
}

// NewMux returns an initialized multiplexor
func NewMux() *Mux {
	return &Mux{routes: make(map[string][]*PatternHandler)}
}

// Add registers the address of a backend service as a handler for an HTTP
// method and URL pattern.
func (mux *Mux) Add(method string, pattern string, address string, service string, c client.KeysAPI) {
	mux.rw.Lock()
	defer mux.rw.Unlock()

	handlers, present := mux.routes[method]
	if !present {
		handlers = make([]*PatternHandler, 0)
	}

	// Search for duplicates.
	for _, handler := range handlers {
		if pattern == handler.Pattern {
			handleDuplicates(*handler, method, pattern, address, service, c)
		}
	}
	// Add a new pattern handler for the pattern and address.

	pSuccess("*********************** New Service Dicovered ***********************\n")
	fmt.Printf("%v %v %v\n", pSuccessInline("Registering Route:"), pMethod(method), pattern)
	fmt.Printf("%v %v\n", pSuccessInline("Route Directed To:"), pBold(strings.Title(strings.Replace(service, "-", " ", -1))))
	fmt.Printf("%v %v\n", pSuccessInline("Service Located At:"), address)

	addresses := []string{address}
	handler := PatternHandler{Pattern: pattern, Addresses: addresses}
	mux.routes[method] = append(handlers, &handler)
}

// Remove unregisters the address of a backend service as a handler for an
// HTTP method and URL pattern.
func (mux *Mux) Remove(method, pattern, address string) {
	mux.rw.Lock()
	defer mux.rw.Unlock()

	handlers, present := mux.routes[method]
	if !present {
		return
	}

	// Find the handler registered for the pattern.
	for i, handler := range handlers {
		if pattern == handler.Pattern {
			// Remove the handler if the address to remove is the only one
			// registered.
			if len(handler.Addresses) == 1 && handler.Addresses[0] == address {
				mux.routes[method] = append(handlers[:i], handlers[i+1:]...)
				return
			}

			// Remove the address from the addresses registered in the
			// handler.
			for j, existingAddress := range handler.Addresses {
				if address == existingAddress {
					handler.Addresses = append(
						handler.Addresses[:j], handler.Addresses[j+1:]...)
					return
				}
			}
		}
	}
}

func handleDuplicates(handler PatternHandler, method string, pattern string, address string, service string, c client.KeysAPI) {
	conflictKey := "/gateway/conflicts/" + service + "/" + os.Getenv("GO_ENV") + "/" + pattern
	var conf conflict
	conf.ExistingAddresses = handler.Addresses
	conf.AttemptedConflict = address
	a, err := json.Marshal(conf)
	if err != nil {
		fmt.Println(err)
		return
	}
	opts := EtcdSetOptions()
	conflictValues := string(a)
	c.Set(context.Background(), conflictKey, conflictValues, opts)
	for _, existingAddress := range handler.Addresses {
		if address == existingAddress {
			duplicateKey := "/gateway/conflicts/duplicates/" + service + "/" + os.Getenv("GO_ENV") + "/" + pattern
			c.Set(context.Background(), duplicateKey, address, opts)
			return
		}
	}
	go func(conflictKey, conflictValues string) {
		pInfo("Not Registering Duplicate: %v %v", conflictKey, conflictValues)
	}(conflictKey, conflictValues)
	// Do not add a new address to an existing pattern handler.
	handler.Addresses = append(handler.Addresses, address)
	return
}

// ServeHTTP dispatches the request to the backend service whose pattern most
// closely matches the request URL.
func (mux *Mux) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// Attempt to match the request against registered patterns and addresses.
	addresses, err := mux.Match(request.Method, request.URL.Path)
	if err != nil {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	incomingParams := request.PostForm.Encode()
	log.Println("INCOMING PARAMS:", incomingParams)
	// Make a request to a random backend service.
	index := rand.Intn(len(*addresses))
	address := (*addresses)[index]

	url := address + strings.Replace(request.URL.Path, "/api", "", 1)
	if len(request.URL.Query()) > 0 {
		url = url + "?" + request.URL.RawQuery
	}
	innerRequest, err := http.NewRequest(request.Method, "http://"+url, request.Body)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	// for k, values := range request.Form {
	// 	for _, v := range values {
	// 		innerRequest.Form.Add(k, v)
	// 	}
	// }
	for header, values := range request.Header {
		for _, value := range values {
			innerRequest.Header.Add(header, value)
		}
	}
	response, err := http.DefaultClient.Do(innerRequest)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Relay the response from the backend service back to the client.
	for header, values := range response.Header {
		for _, value := range values {
			writer.Header().Add(header, value)
		}
	}
	writer.WriteHeader(response.StatusCode)
	body := bytes.NewBufferString("")
	body.ReadFrom(response.Body)
	writer.Write(body.Bytes())
}

// Match finds backend service addresses capable of handling a request for the
// given HTTP method and URL pattern.  An error is returned if no addresses
// are registered for the given HTTP method and URL pattern.
func (mux *Mux) Match(method, pattern string) (*[]string, error) {
	mux.rw.RLock()
	defer mux.rw.RUnlock()

	handlers, present := mux.routes[method]
	if present {
		for _, handler := range handlers {
			if handler.Match(pattern) {
				return &handler.Addresses, nil
			}
		}
	}
	return nil, errors.New("No matching address")
}

// Match returns true if this handler is a match for path.
func (handler *PatternHandler) Match(path string) bool {
	var i, j int
	for i < len(path) {
		switch {
		case j == len(handler.Pattern) && handler.Pattern[j-1] == '/':
			return true
		case j >= len(handler.Pattern):
			return false
		case handler.Pattern[j] == ':':
			j = handler.find(handler.Pattern, '/', j)
			i = handler.find(path, '/', i)
		case path[i] == handler.Pattern[j]:
			i++
			j++
		default:
			return false
		}
	}
	if j != len(handler.Pattern) {
		return false
	}
	return true
}

// Find searches text for char, starting at startIndex, and returns the index
// of the next instance of char.  startIndex is returned if no instance of
// char is found.
func (handler *PatternHandler) find(text string, char byte, startIndex int) int {
	j := startIndex
	for j < len(text) && text[j] != char {
		j++
	}
	return j
}
