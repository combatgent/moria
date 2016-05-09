package moria

import (
	"errors"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/etcd/client"
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

// OXY UTILS COMPAT TESTING
const (
	XForwardedProto    = "X-Forwarded-Proto"
	XForwardedFor      = "X-Forwarded-For"
	XForwardedHost     = "X-Forwarded-Host"
	XForwardedServer   = "X-Forwarded-Server"
	Connection         = "Connection"
	KeepAlive          = "Keep-Alive"
	ProxyAuthenticate  = "Proxy-Authenticate"
	ProxyAuthorization = "Proxy-Authorization"
	Te                 = "Te" // canonicalized version of "TE"
	Trailers           = "Trailers"
	TransferEncoding   = "Transfer-Encoding"
	Upgrade            = "Upgrade"
	ContentLength      = "Content-Length"
)

type handlerContext struct {
	errHandler ErrorHandler
	log        Logger
}
type ErrorHandler interface {
	ServeHTTP(w http.ResponseWriter, req *http.Request, err error)
}

var DefaultHandler ErrorHandler = &StdHandler{}

type StdHandler struct {
}

func (e *StdHandler) ServeHTTP(w http.ResponseWriter, req *http.Request, err error) {
	statusCode := http.StatusInternalServerError
	if e, ok := err.(net.Error); ok {
		if e.Timeout() {
			statusCode = http.StatusGatewayTimeout
		} else {
			statusCode = http.StatusBadGateway
		}
	} else if err == io.EOF {
		statusCode = http.StatusBadGateway
	}
	w.WriteHeader(statusCode)
	w.Write([]byte(http.StatusText(statusCode)))
}

type ErrorHandlerFunc func(http.ResponseWriter, *http.Request, error)

// ServeHTTP calls f(w, r).
func (f ErrorHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request, err error) {
	f(w, r, err)
}

var NullLogger Logger = &NOPLogger{}

// Logger defines a simple logging interface
type Logger interface {
	Infof(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

type FileLogger struct {
	info  *log.Logger
	warn  *log.Logger
	error *log.Logger
}

func NewFileLogger(w io.Writer, lvl LogLevel) *FileLogger {
	l := &FileLogger{}
	flag := log.Ldate | log.Ltime | log.Lmicroseconds
	if lvl <= INFO {
		l.info = log.New(w, "INFO: ", flag)
	}
	if lvl <= WARN {
		l.warn = log.New(w, "WARN: ", flag)
	}
	if lvl <= ERROR {
		l.error = log.New(w, "ERR: ", flag)
	}
	return l
}

func (f *FileLogger) Infof(format string, args ...interface{}) {
	if f.info == nil {
		return
	}
	f.info.Printf(format, args...)
}

func (f *FileLogger) Warningf(format string, args ...interface{}) {
	if f.warn == nil {
		return
	}
	f.warn.Printf(format, args...)
}

func (f *FileLogger) Errorf(format string, args ...interface{}) {
	if f.error == nil {
		return
	}
	f.error.Printf(format, args...)
}

type NOPLogger struct {
}

func (*NOPLogger) Infof(format string, args ...interface{}) {

}
func (*NOPLogger) Warningf(format string, args ...interface{}) {
}

func (*NOPLogger) Errorf(format string, args ...interface{}) {
}

func (*NOPLogger) Info(string) {

}
func (*NOPLogger) Warning(string) {
}

func (*NOPLogger) Error(string) {
}

type LogLevel int

const (
	INFO = iota
	WARN
	ERROR
)

//

// Mux is an HTTP request multiplexer.  It matches the URL of
// each incoming request against a list of registered patterns to find the
// service that can respond to it and proxies the request to the appropriate
// backend.  Pattern matching logic is based on pat.go.
// ** FROM https://github.com/jkakar/switchboard
type Mux struct {
	rw           sync.RWMutex                 // Synchronize access to routes map.
	routes       map[string][]*PatternHandler // Patterns mapped to backend services.
	roundTripper http.RoundTripper
	ctx          handlerContext
}

// NewMux returns an initialized multiplexor
func NewMux() *Mux {
	return &Mux{routes: make(map[string][]*PatternHandler), roundTripper: http.DefaultTransport}
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
			handleDuplicates(handler, method, pattern, address, service, c)
			return
		}
	}
	// Add a new pattern handler for the pattern and address.
	log.Printf("\n>**************************** New Service Dicovered ****************************\n>\t%v %v %v\n>\t%v %v\n>\t%v %v\n", pSuccessInline("Registering Route:"), pMethod(method), pattern, pSuccessInline("Route Directed To:"), pBold(strings.Title(strings.Replace(service, "-", " ", -1))), pSuccessInline("Service Located At:"), address)
	addresses := []string{address}
	handler := PatternHandler{Pattern: pattern, Addresses: addresses}
	mux.routes[method] = append(handlers, &handler)
}

func handleDuplicates(handler *PatternHandler, method string, pattern string, address string, service string, c client.KeysAPI) {
	for _, existingAddress := range handler.Addresses {
		if strings.Compare(address, existingAddress) == 0 {
			return
		}
	}
	// If address doesnt exist for pattern append to handler
	handler.Addresses = append(handler.Addresses, address)
	return
}

// Remove unregisters the address of a backend service as a handler for an
// HTTP method and URL pattern.
func (mux *Mux) Remove(method, pattern, address, service string) {
	pattern = "/api" + pattern
	mux.rw.Lock()
	defer mux.rw.Unlock()
	handlers, present := mux.routes[method]
	if !present {
		log.Printf("\n>\tFAILING Pattern To Be Deleted: %v \n", pattern)
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
		} else {
			log.Printf("\n>\tPATTERN: %v does not match %v", pattern, handler.Pattern)
		}
	}
}

// ServeHTTP dispatches the request to the backend service whose pattern most
// closely matches the request URL.
func (mux *Mux) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	start := time.Now().UTC()
	// Create address string
	var address string
	// Attempt to match the request against registered patterns and addresses.
	err := findHost(mux, request, writer, &address)
	if err != nil {
		log.Printf("%v", pDisappointedInline("Invalid URL Pattern"))
		return
	}
	// Make new request copy old stuff over
	response, err := mux.roundTripper.RoundTrip(generateInnerRequest(request, address))
	// Execute request
	//response, err := http.DefaultClient.Do(innerRequest)
	if err != nil {
		log.Printf("____________________________ INTERNAL ERROR _______________________________\n***************************************\n>\t%+v\n************************************\n", err)
		mux.ctx.log.Errorf("Error forwarding to %v, err: %v", request.URL, err)
		mux.ctx.errHandler.ServeHTTP(writer, request, err)
		return
	}
	if request.TLS != nil {
		mux.ctx.log.Infof("Round trip: %v, code: %v, duration: %v tls:version: %x, tls:resume:%t, tls:csuite:%x, tls:server:%v",
			request.URL, response.StatusCode, time.Now().UTC().Sub(start),
			request.TLS.Version,
			request.TLS.DidResume,
			request.TLS.CipherSuite,
			request.TLS.ServerName)
	} else {
		mux.ctx.log.Infof("Round trip: %v, code: %v, duration: %v",
			request.URL, response.StatusCode, time.Now().UTC().Sub(start))
	}
	// Relay the response from the backend service back to the client.
	CopyHeaders(writer.Header(), response.Header)
	writer.WriteHeader(response.StatusCode)
	written, err := io.Copy(writer, response.Body)
	defer response.Body.Close()
	if err != nil {
		mux.ctx.log.Errorf("Error copying upstream response Body: %v", err)
		mux.ctx.errHandler.ServeHTTP(writer, request, err)
		return
	}

	if written != 0 {
		writer.Header().Set(ContentLength, strconv.FormatInt(written, 10))
	}

}

//CopyHeaders adds headers to a response
func CopyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
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
	innerRequest.Close = false
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
