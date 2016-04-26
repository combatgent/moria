package moria

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/etcd/client"
	"github.com/fatih/color"
	"golang.org/x/net/context"
)

// EtcdAPI is an all in one etcd configuration method.
// Config process adapted from https://github.com/coreos/etcd/tree/master/client
func EtcdAPI() client.KeysAPI {
	urls := EtcdURL()
	cfg := EtcdConfig(urls)
	c, err := client.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	return client.NewKeysAPI(c)
}

// EtcdURL generates an array of url string for etcd api client to consume
func EtcdURL() []string {
	peers := os.Getenv("ETCD_CLIENT_PEERS")
	return strings.Split(peers, ",")
}

// EtcdConfig creates a config objectfor the etcd api client object
func EtcdConfig(urls []string) client.Config {
	customTransport := generateTransport()
	c := client.Config{
		Endpoints:               []string{urls[0], urls[1], urls[2]},
		Transport:               customTransport,
		HeaderTimeoutPerRequest: time.Second * 5,
	}
	c.Username = os.Getenv("ETCD_USERNAME")
	c.Password = os.Getenv("ETCD_PASSWORD")
	return c
}

func generateTransport() client.CancelableTransport {
	tlsConfig := &tls.Config{RootCAs: x509.NewCertPool()}
	transport := &http.Transport{TLSClientConfig: generateTLSConfig(tlsConfig)}
	var CustomTransport client.CancelableTransport = transport
	return CustomTransport
}

func generateTLSConfig(tlsConfig *tls.Config) *tls.Config {
	var pemData []byte
	if os.Getenv("GO_ENV") != "production" {
		pemData = getPEMBytesFromFile()
	} else {
		pemData = []byte(os.Getenv("ETCD_CA_STRING"))
	}
	return appendPEM(tlsConfig, pemData)
}

func appendPEM(tlsConfig *tls.Config, pemData []byte) *tls.Config {
	ok := tlsConfig.RootCAs.AppendCertsFromPEM(pemData)
	if !ok {
		panic("Couldn't load PEM data")
	}
	return tlsConfig
}

func getPEMBytesFromFile() []byte {
	pemData, err := ioutil.ReadFile(os.Getenv("ETCD_CA_CERT_PATH"))
	if err != nil {
		panic("invalid path to root certificate")
	}
	return pemData
}

// EtcdGetOptions returns an options struct for the etcd get keys request
func EtcdGetOptions() *client.GetOptions {
	return &client.GetOptions{
		Recursive: true,
		Sort:      false,
		Quorum:    false}
}

// EtcdGetDirectOptions returns an options struct for the etcd get keys request
func EtcdGetDirectOptions() *client.GetOptions {
	return &client.GetOptions{
		Recursive: false,
		Sort:      false,
		Quorum:    false}
}

// EtcdSetOptions returns an options struct for the etcd set keys request
func EtcdSetOptions() *client.SetOptions {
	return &client.SetOptions{
		PrevIndex: 0,
		PrevExist: client.PrevNoExist,
		Refresh:   false,
		Dir:       false}
}

// EtcdWatcherOptions returns an options struct for the etcd Watcher object
func EtcdWatcherOptions(index uint64) *client.WatcherOptions {
	return &client.WatcherOptions{
		AfterIndex: index,
		Recursive:  true}
}

func checkEtcdErrors(err error) {
	if err != nil {
		if err == context.Canceled {
			ctxCancelled(err)
		} else if err == context.DeadlineExceeded {
			ctxDeadlineExceeded(err)
		} else if cerr, ok := err.(*client.ClusterError); ok {
			clusterError(cerr)
		} else {
			badCluster(err)
		}
	}
}

// MatchEnv checks that this key has values for the appropriate env
func MatchEnv(k string) bool {

	key := strings.TrimPrefix(k, "/")
	keyEnv := strings.Split(key, "/")
	if len(keyEnv) > 3 {
		if (keyEnv[2] == os.Getenv("GO_ENV")) && (keyEnv[3] == "routes") {
			return true
		}
	}
	return false
}

// ID returns service ID
func ID(k string) string {
	key := strings.TrimPrefix(k, "/")
	id := strings.Split(key, "/")[1]
	return id
}

// Host returns service Host
func Host(k string) string {
	key := strings.TrimPrefix(k, "/")
	keys := strings.Split(key, "/")
	host := ""
	for i, w := range keys {
		if i < (len(keys) - 1) {
			host += w + "/"
		} else {
			host += "hosts"
		}
	}
	return host
}

// EtcdRoute is a route that uses grape export url patterns to store json
type EtcdRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// ERRORS
// KeyNotFoundError error type for missing etcd info
type KeyNotFoundError struct {
	Message string
}

func ctxCancelled(err error) {
	color.Set(color.FgRed)
	log.Printf(
		`*************************** ERROR *******************************
  CTX WAS CANCELED BY ANOTHER ROUTINE DETAILS BELOW:
	%+v`, err.Error())

}

func ctxDeadlineExceeded(err error) {
	color.Set(color.FgRed)
	log.Printf(
		`*************************** ERROR *******************************
  CTX WAS ATTACHED WITH A DEADLINE AND IT EXCEEDED DETAILS BELOW:
	%+v`, err.Error())

}

func clusterError(err error) {
	color.Set(color.FgRed)
	log.Printf(
		`*************************** ERROR *******************************
  PROCESS (cerr.Errors) DETAILS BELOW:
	%+v`, err.Error())
	panic(&KeyNotFoundError{Message: err.Error()})
}

func badCluster(err error) {
	color.Set(color.FgRed)
	log.Printf(
		`*************************** ERROR *******************************
  DETAILS BELOW:\n%+v`, err.Error())

}
