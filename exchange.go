package moria

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

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
		checkNodes(exchange, response.Node)
	}
	// We want to watch changes *after* this one.
	exchange.waitIndex = response.Index + 1
	return nil
}

func checkNodes(exchange *Exchange, node *client.Node) {

	for _, node := range node.Nodes {
		pInfo("Checking Key: %v\n", node.Key)
		if MatchEnv(node.Key) {
			defer func() {
				if perr := recover(); perr != nil {
					var ok bool
					perr, ok = perr.(error)
					if !ok {
						fmt.Errorf("Panicking: %v", perr)
					}
				}
			}()
			pSuccess("Found Matching Key: %v\nWith Value:%v\n", node.Key, node.Value)
			service := exchange.load(node.Value)
			service.ID = ID(node.Key)
			host := Host(node.Key)
			resp, err := exchange.client.Get(context.Background(), host, EtcdGetDirectOptions())
			CheckEtcdErrors(err)
			for _, node := range resp.Node.Nodes {
				service.Address = node.Value
				exchange.Register(service)
			}
		}
		if node.Nodes.Len() > 0 {
			checkNodes(exchange, node)
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
				checkNodes(exchange, response.Node)
			} else if response.Action == "delete" {
				node := response.Node
				pInfo("Checking Key: %v\n", node.Key)
				if MatchEnv(response.Node.Key) {
					defer func() {
						if perr := recover(); perr != nil {
							var ok bool
							perr, ok = perr.(error)
							if !ok {
								fmt.Errorf("Panicking: %v", perr)
							}
						}
					}()
					service := exchange.services[ID(response.Node.Key)]
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
			exchange.mux.Add(method, pattern, service.Address, service.ID, exchange.client)
		}
	}
}

// Unregister removes routes exposed by a service from the ExchangeServeMux.
func (exchange *Exchange) Unregister(service *ServiceRecord) {
	for method, patterns := range service.Routes {
		for _, pattern := range patterns {
			exchange.mux.Remove(method, pattern, service.Address)
		}
	}
}
