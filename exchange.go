package moria

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
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
				log.Printf("Panicking: %v", perr)
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
		go func(exchange *Exchange, node *client.Node) {
			registerNodes(exchange, node)
		}(exchange, response.Node)

	}
	// We want to watch changes *after* this one.
	exchange.waitIndex = response.Index + 1

	return nil
}

func registerNodes(exchange *Exchange, node *client.Node) {
	for _, n := range node.Nodes {
		registerNode(exchange, n)
	}
}

func registerNode(exchange *Exchange, n *client.Node) {
	if MatchEnv(n.Key) {
		defer func() {
			if perr := recover(); perr != nil {
				var ok bool
				perr, ok = perr.(error)
				if !ok {
					log.Printf("Panicking: %v", perr)
				}
			}
		}()
		service := exchange.load(n.Value)
		service.ID = ID(n.Key)
		host := Host(n.Key)
		resp, err := exchange.client.Get(context.Background(), host, EtcdGetDirectOptions())
		CheckEtcdErrors(err)
		for _, rn := range resp.Node.Nodes {
			service.Address = rn.Value
			exchange.Register(service)
		}
	} else if MatchHostsEnv(n.Key) {
		defer func() {
			if perr := recover(); perr != nil {
				var ok bool
				perr, ok = perr.(error)
				if !ok {
					log.Printf("Panicking: %v", perr)
				}
			}
		}()
		log.Printf("\n>\tFound Matching Hosts Key:\n>\t%v\n", pSuccessInline(n.Key))
		if service, ok := exchange.services[ID(n.Key)]; ok {
			service.Address = n.Value
			exchange.Register(service)
		}
	}
	if n.Nodes.Len() > 0 {
		go func(exchange *Exchange, node *client.Node) {
			registerNodes(exchange, n)
		}(exchange, n)
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
		options := EtcdGetOptions()
		ctx := context.TODO()
		select {
		case response := <-receiver:
			if response.Action == "set" {
				splitKeys := strings.Split(response.Node.Key, "/")
				if splitKeys[len(splitKeys)-1] == "routes" {
					log.Println("\n\t\t\t\tModifying Routes\n********************************************************************************")
					getRootNode(response.Node.Key)
					resp, _ := exchange.client.Get(ctx, getRootNode(response.Node.Key), options)
					registerNode(exchange, resp.Node)
				} else {
					log.Println("\n\t\t\t\tModifying Hosts\n********************************************************************************")
					go func(exchange *Exchange, node *client.Node) {
						registerNode(exchange, node)
					}(exchange, response.Node)
				}
			} else if response.Action == "delete" {
				go func(exchange *Exchange, prevNode *client.Node) {
					log.Println("\n\t\t\t\tDeleting Hosts\n********************************************************************************")
					unregisterNode(exchange, prevNode)
				}(exchange, response.PrevNode)
			} else {
				log.Printf("\n>\tUNCAUGHT RESPONSE ACTION: %v", resp.Action)
			}
		}
	}
}

func getRootNode(key string) string {

	rootkey := ""
	splitKeys := strings.Split(key, "/")
	for i, v := range splitKeys {
		if i < (len(splitKeys) - 1) {
			rootkey += v + "/"
		}
	}
	return rootkey
}

func getIPAddress() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		os.Stderr.WriteString("Oops: " + err.Error() + "\n")
		os.Exit(1)
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				os.Stdout.WriteString(ipnet.IP.String() + "\n")
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
func gatewayNamespace() (string, string) {
	var host, uName, outputUName, outputHost string
	var hostErr, uNameErr error
	outputUName, uNameErr = os.Hostname()
	if !strings.Contains(outputUName, ".local") {
		outputHost = getIPAddress()
		if uNameErr != nil {
			log.Printf("\n>\tUnable To Publish Dyno UName\n>\t%+v", hostErr)
		}
		host = string(outputHost)
		uName = "/gateway/environments/" + os.Getenv("GO_ENV") + "/" + string(outputUName)
		log.Printf("\n>\tUNAME:\n>%v\n>\tHOST ADDRESS:\n>\t%v", uName, host)
	} else {
		host = "127.0.0.1"
		uName = "/gateway/environments/" + os.Getenv("GO_ENV") + "/" + string(outputUName)
		log.Printf("\n>\tUNAME:\n>%v\n>\tHOST ADDRESS:\n>\t%v", uName, host)
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
		  for _, method := range []string{"GET", "PUT","POST","DELETE","PATCH"}{
				if arr, ok := exchange.mux.routes[method]; ok{
					for _, handler := arr {
						log.Printf("\n>\tHANDLER: %+v\n", handler)
					}
				}
			}
			resp, err := exchange.client.Set(context.Background(), key, address, opts)
			if err != nil {
				log.Println("ERROR: ", err)
			} else {
				log.Printf("\n>\t%v \"%v\"\n>\t%v%v", pInfoInline("Success Gateway Alive At:"), pInfoInline(address), pInfoInline("Services May Locate This Gateway At The Key Provided Below\n>\tGATEWAY_KEY="), pInfoInline(resp.Node.Key))
			}
		}
	}()
}

func unregisterNodes(exchange *Exchange, node *client.Node) {
	for _, n := range node.Nodes {
		unregisterNode(exchange, n)
	}
}

func unregisterNode(exchange *Exchange, n *client.Node) {
	pInfo("\n>\tUnregistering Key: %v\n", n.Key)
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
			service.Address = n.Value
			exchange.Unregister(service)
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
			log.Printf("\n>\nREMOVING PATTERN\n>\tPATTERN DETAILS: %v %v\n>\tSERVICE DETAILS: %v %v", method, pattern, service.Address, service.ID)
			exchange.mux.Remove(method, pattern, service.Address, service.ID)
		}
	}
}
