package moria

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
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
			return
		}
	}
	// Add a new pattern handler for the pattern and address.
	if strings.Compare(address, "") == 0 {
		log.Printf("\n>\t%v\n>\tAddress: %v", pDisappointedInline("Tried to register \"\" as a valid host please avoid if at all possible"), address)
		log.Printf("\n>********************** New Service Could Not Be Dicovered *********************\n>\t%v %v %v\n>\t%v %v\n>\t%v %v\n", pDisappointedInline("Not Registering Route:"), pMethod(method), pattern, pDisappointedInline("Route Could Not Be Directed To:"), pBold(strings.Title(strings.Replace(service, "-", " ", -1))), pDisappointedInline("Service Could Not Have Been Located At:"), address)
	} else {
		log.Printf("\n>**************************** New Service Dicovered ****************************\n>\t%v %v %v\n>\t%v %v\n>\t%v %v\n", pSuccessInline("Registering Route:"), pMethod(method), pattern, pSuccessInline("Route Directed To:"), pBold(strings.Title(strings.Replace(service, "-", " ", -1))), pSuccessInline("Service Located At:"), address)
		addresses := []string{address}
		handler := PatternHandler{Pattern: pattern, Addresses: addresses}
		mux.routes[method] = append(handlers, &handler)
	}
}

// Remove unregisters the address of a backend service as a handler for an
// HTTP method and URL pattern.
func (mux *Mux) Remove(method, pattern, address, service string) {
	pattern = "/api" + pattern
	mux.rw.Lock()
	defer mux.rw.Unlock()
	handlers, present := mux.routes[method]
	if !present {
		return
	}
	// Find the handler registered for the pattern.
	for i, handler := range handlers {
		if strings.Compare(pattern, handler.Pattern) == 0 {
			// Remove the handler if the address to remove is the only one
			// registered.
			log.Println("*********************** Unregisterring Service Host ***********************")
			log.Printf("\n>\t%v %v %v\n", pSuccessInline("Unregistering Route:"), pMethod(method), pattern)
			log.Printf("\n>\t%v %v\n", pSuccessInline("Service No Longer Located At:"), address)
			log.Printf("\n>\tPattern To Be Deleted: %v \n>\tPattern Stored: %v\n", pattern, handler.Pattern)
			if len(handler.Addresses) == 1 && handler.Addresses[0] == address {
				log.Printf("\n>\t%v %v\n>\tRemoved Handler Entirely", pSuccessInline("Route No Longer Directed To:"), pBold(strings.Title(strings.Replace(service, "-", " ", -1))))
				mux.routes[method] = append(handlers[:i], handlers[i+1:]...)
				return
			}

			// Remove the address from the addresses registered in the
			// handler.
			for j, existingAddress := range handler.Addresses {
				if address == existingAddress {
					log.Printf("\n>\t%v %v\n>\tRemoved Host From Handler Only", pSuccessInline("Route No Longer Directed To:"), pBold(strings.Title(strings.Replace(service, "-", " ", -1))))

					handler.Addresses = append(handler.Addresses[:j], handler.Addresses[j+1:]...)
					return
				}
			}
		}
	}
}

func handleDuplicates(handler PatternHandler, method string, pattern string, address string, service string, c client.KeysAPI) {
	conflictKey := "/gateway/conflicts/" + service + "/" + os.Getenv("VINE_ENV") + "/" + pattern
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
		if strings.Compare(address, existingAddress) == 0 {
			duplicateKey := "/gateway/conflicts/duplicates/" + service + "/" + os.Getenv("VINE_ENV") + "/" + pattern
			c.Set(context.Background(), duplicateKey, address, opts)
			return
		} else {
			log.Printf("\n>\t%v!=%v", address, existingAddress)
		}
	}
	//log.Printf("\n>\tADDING PATTERN\n>\tPATTERN DETAILS: %v %v\n>\tSERVICE DETAILS: %v %v", method, pattern, service.Address, service.ID)
	// Add a new address to an existing pattern handler.
	// If it is not ""
	if address != "" {
		handler.Addresses = append(handler.Addresses, address)
	} else {
		log.Printf("\n>\t%v\n>\tAddress: %v", pDisappointedInline("Tried to register \"\" as a valid host please avoid if at all possible"), address)
		log.Printf("\n>********************** New Service Could Not Be Dicovered *********************\n>\t%v %v %v\n>\t%v %v\n>\t%v %v\n", pDisappointedInline("Not Registering Route:"), pMethod(method), pattern, pDisappointedInline("Route Could Not Be Directed To:"), pBold(strings.Title(strings.Replace(service, "-", " ", -1))), pDisappointedInline("Service Could Not Have Been Located At:"), address)
	}
	return
}

// ServeHTTP dispatches the request to the backend service whose pattern most
// closely matches the request URL.
func (mux *Mux) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// Create address string
	var address string
	// Attempt to match the request against registered patterns and addresses.
	err := findHost(mux, request, writer, &address)
	if err != nil {
		log.Printf("%v", pDisappointedInline("Invalid URL Pattern"))
		return
	}
	// Make new request copy old stuff over
	innerRequest := generateInnerRequest(request, address)
	// Execute request
	response, err := http.DefaultClient.Do(innerRequest)
	if err != nil {
		log.Printf("____________________________ INTERNAL ERROR _______________________________\n%+v", err)
		// TODO: Add JSON response here for UI
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Header().Set("Content-Type", "application/json")
		jsonStr := `[{"error":"500 Internal Server Error"},{"status":500}]`
		writer.Write([]byte(jsonStr))
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

// CopyURL provides update safe copy by avoiding shallow copying User field
func CopyURL(i *url.URL) *url.URL {
	out := *i
	if i.User != nil {
		out.User = &(*i.User)
	}
	return &out
}

func generateInnerRequest(request *http.Request, address string) *http.Request {
	innerRequest := new(http.Request)
	*innerRequest = *request // includes shallow copies of maps, but we handle this below
	innerRequest.URL = CopyURL(request.URL)
	innerRequest.URL.Scheme = "http"
	innerRequest.URL.Host = address
	innerRequest.URL.Path = strings.Replace(request.URL.Path, "/api", "", 1)
	innerRequest.URL.RawQuery = request.URL.RawQuery
	innerRequest.RequestURI = ""
	innerRequest.Header = make(http.Header)
	for header, values := range request.Header {
		for _, value := range values {
			innerRequest.Header.Add(header, value)
		}
	}
	return innerRequest
}

func findHost(mux *Mux, request *http.Request, writer http.ResponseWriter, address *string) error {
	addresses, err := mux.Match(request.Method, request.URL.Path)
	// TODO: Add JSON response here
	if err != nil {
		writer.WriteHeader(http.StatusNotFound)
		writer.Header().Set("Content-Type", "application/json")
		jsonStr := `[{"error":"404 Status Not Found"},{"status":404}]`
		writer.Write([]byte(jsonStr))
		return errors.New("No Matching Pattern")
	}
	// Make a request to a random backend service.
	index := rand.Intn(len(*addresses))
	*address = (*addresses)[index]
	return nil
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
				log.Println("Found MatchingPattern", pattern)
				return &handler.Addresses, nil
			}
		}
	} else {
		return nil, errors.New("No matching address")
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
