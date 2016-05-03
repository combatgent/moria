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
		registerNode(exchange, n)
	}
}

func registerNode(exchange *Exchange, n *client.Node) {
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
	} else if MatchHostsEnv(n.Key) {
		defer func() {
			if perr := recover(); perr != nil {
				var ok bool
				perr, ok = perr.(error)
				if !ok {
					fmt.Errorf("Panicking: %v", perr)
				}
			}
		}()
		pSuccess("Found Matching Hosts Key: %v\n", n.Key)
		if service, ok := exchange.services[ID(n.Key)]; ok {
			service.Address = n.Value
			log.Println("Registering new Host")
			exchange.Register(service)
		}
	}
	if n.Nodes.Len() > 0 {
		registerNodes(exchange, n)
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
			log.Printf("OUTER: Got Response: %v\nOUTER: Executed on node key: %v", r.Action, r.Node.Key)
			if err != nil {
				color.Set(color.FgRed)
				log.Println(err)
				color.Unset()
			}
			receiver <- r
		}
	}()
	for {
		log.Println("Running For Loop")
		options := EtcdGetOptions()
		ctx := context.TODO()
		select {
		case response := <-receiver:
			log.Printf("INNER: Got Response: %v\nINNER: Executed on node key: %v", response.Action, response.Node.Key)
			if response.Action == "set" {
				log.Println("********************************************************************************")
				splitKeys := strings.Split(response.Node.Key, "/")
				if splitKeys[len(splitKeys)-1] == "routes" {
					log.Println("Modifying Routes")
					log.Println("********************************************************************************")
					getRootNode(response.Node.Key)
					resp, _ := exchange.client.Get(ctx, getRootNode(response.Node.Key), options)
					registerNode(exchange, resp.Node)
				} else {
					log.Println("Modifying Hosts")
					log.Println("********************************************************************************")
					registerNode(exchange, response.Node)
				}
			} else if response.Action == "delete" {
				unregisterNodes(exchange, response.Node)
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
	log.Println("Outside:", outputUName)
	if !strings.Contains(outputUName, ".local") {
		outputHost = getIPAddress()
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
