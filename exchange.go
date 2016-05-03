package moria

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/coreos/etcd/client"
	"github.com/fatih/color"
	"golang.org/x/net/context"
)

// Exchange watches for service changes in etcd and update an
// ExchangeServeMux.
type Exchange struct {
	namespace string                    // The root directory in etcd for services.
	client    client.KeysAPI            // The etcd client.
	mux       *Mux                      // The serve mux to keep in sync with etcd.
	waitIndex uint64                    // Wait index to use when watching etcd.
	services  map[string]*ServiceRecord // Currently connected services.
}

// NewExchange creates a new exchange configured to watch for changes in a
// given etcd directory.
func NewExchange(namespace string, client client.KeysAPI, mux *Mux) *Exchange {
	return &Exchange{
		namespace: namespace,
		client:    client,
		mux:       mux,
		services:  make(map[string]*ServiceRecord)}
}

// Init fetches service information from etcd and initializes the exchange.
func (exchange *Exchange) Init() error {
	defer func() {
		if perr := recover(); perr != nil {
			var ok bool
			perr, ok = perr.(error)
			if !ok {
				fmt.Errorf("Panicking: %v", perr)
			}
		}
	}()
	options := EtcdGetOptions()
	ctx := context.TODO()
	response, err := exchange.client.Get(ctx, exchange.namespace, options)
	if err != nil {
		CheckEtcdErrors(err)
	}
	if response.Node.Nodes.Len() > 0 {
		registerNodes(exchange, response.Node)
	}
	// We want to watch changes *after* this one.
	exchange.waitIndex = response.Index + 1
	return nil
}

func registerNodes(exchange *Exchange, node *client.Node) {
	for _, n := range node.Nodes {
		pInfo("Checking &Key: %v\n", n.Key)
		if MatchEnv(n.Key) {
			defer func() {
				if perr := recover(); perr != nil {
					var ok bool
					perr, ok = perr.(error)
					if !ok {
						fmt.Errorf("Panicking: %v", perr)
					}
				}
			}()
			pSuccess("Found Matching Key: %v\n", n.Key)
			service := exchange.load(n.Value)
			service.ID = ID(n.Key)
			host := Host(n.Key)
			resp, err := exchange.client.Get(context.Background(), host, EtcdGetDirectOptions())
			CheckEtcdErrors(err)
			for _, rn := range resp.Node.Nodes {
				service.Address = rn.Value
				exchange.Register(service)
			}
		}
		if n.Nodes.Len() > 0 {
			registerNodes(exchange, n)
		}
	}
}

// Watch observes changes in etcd and registers and unregisters services, as
// necessary, with the ExchangeServeMux.  This blocking call will terminate
// when a value is received on the stop channel.
func (exchange *Exchange) Watch() {
	ns := Namespace()
	opts := EtcdWatcherOptions(exchange.waitIndex)
	watcher := exchange.client.Watcher(ns, opts)
	receiver := make(chan *client.Response)
	defer close(receiver)
	go func() {
		for {
			r, err := watcher.Next(context.Background())
			if err != nil {
				color.Set(color.FgRed)
				log.Println(err)
				color.Unset()
			}
			receiver <- r
		}
	}()
	for {
		select {
		case response := <-receiver:
			if response.Action == "set" {
				registerNodes(exchange, response.Node)
			} else if response.Action == "delete" {
				unregisterNodes(exchange, response.Node)
			}
		}
	}
}

func gatewayNamespace() (string, string) {
	var host, uName, outputUName string
	var hostErr, uNameErr error
	var outputHost []byte
	outputUName, uNameErr = os.Hostname()
	log.Println("Outside:", outputUName)
	if !strings.Contains(outputUName, "Darwin") {
		outputHost, hostErr = exec.Command("$(ip -4 -o addr show dev eth1 | awk '{print $4}' | cut -d/ -f1)").Output()
		if hostErr != nil {
			log.Println("Unable to publish dyno address", hostErr)
		} else if uNameErr != nil {
			log.Println("Unable to publish dyno uName", hostErr)
		}
		host = string(outputHost)
		uName = "/gateway/environments/" + os.Getenv("GO_ENV") + "/" + string(outputUName)
		log.Println("---UNAME---", uName)
	} else {
		host = "127.0.0.1"
		uName = "/gateway/environments/" + os.Getenv("GO_ENV") + "/" + string(outputUName)
		log.Println("---UNAME---", uName)
	}
	return strings.Join([]string{host, ":", os.Getenv("PORT")}, ""), uName
}

func gatewaySetOpts() *client.SetOptions {
	opts := &client.SetOptions{}
	opts.Refresh = true
	opts.TTL = time.Second * 30
	return opts
}

func (exchange *Exchange) PublishLocation() {
	address, key := gatewayNamespace()
	opts := gatewaySetOpts()
	go func() {
		for {
			time.Sleep(5 * time.Second)
			resp, err := exchange.client.Set(context.Background(), key, address, opts)
			if err != nil {
				log.Println("ERROR: ", err)
			} else {
				log.Println("SUCCESS: Key", resp.Node.Key, "updated.\n Gateway alive at: \"", address, "\"")
			}
			log.Println("Checking Gateway...")
		}
	}()
}

func unregisterNodes(exchange *Exchange, node *client.Node) {
	for _, n := range node.Nodes {
		pInfo("Unregistering Key: %v\n", n.Key)
		if MatchHostsEnv(n.Key) {
			defer func() {
				if perr := recover(); perr != nil {
					var ok bool
					perr, ok = perr.(error)
					if !ok {
						fmt.Errorf("Panicking: %v", perr)
					}
				}
			}()
			if service, ok := exchange.services[ID(n.Key)]; ok {
				host := Host(n.Key)
				resp, err := exchange.client.Get(context.Background(), host, EtcdGetDirectOptions())
				CheckEtcdErrors(err)
				for _, respNode := range resp.Node.Nodes {
					if n.Value == respNode.Value {
						service.Address = respNode.Value
						exchange.Unregister(service)
					}
				}
			}
		} else if MatchEnv(n.Key) {
			defer func() {
				if perr := recover(); perr != nil {
					var ok bool
					perr, ok = perr.(error)
					if !ok {
						fmt.Errorf("Panicking: %v", perr)
					}
				}
			}()
			if service, ok := exchange.services[ID(n.Key)]; ok {
				host := Host(n.Key)
				resp, err := exchange.client.Get(context.Background(), host, EtcdGetDirectOptions())
				CheckEtcdErrors(err)
				for _, respNode := range resp.Node.Nodes {
					service.Address = respNode.Value
					exchange.Unregister(service)
				}
			}
		}
	}
}

func (exchange *Exchange) load(js string) *ServiceRecord {
	var routes []EtcdRoute
	var s ServiceRecord
	json.Unmarshal(bytes.NewBufferString(js).Bytes(), &routes)
	s.GenerateRecord(routes)
	return &s
}

// Register adds routes exposed by a service to the ExchangeServeMux.
func (exchange *Exchange) Register(service *ServiceRecord) {
	exchange.services[service.ID] = service
	for method, patterns := range service.Routes {
		for _, pattern := range patterns {
			pattern = "/api" + pattern
			exchange.mux.Add(method, pattern, service.Address, service.ID, exchange.client)
		}
	}
}

// Unregister removes routes exposed by a service from the ExchangeServeMux.
func (exchange *Exchange) Unregister(service *ServiceRecord) {
	for method, patterns := range service.Routes {
		for _, pattern := range patterns {
			exchange.mux.Remove(method, pattern, service.Address, service.ID)
		}
	}
}
