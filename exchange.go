package moria

import (
	"bytes"
	"encoding/json"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
)

// Exchange watches for service changes in etcd and update an
// ExchangeServeMux.
type Exchange struct {
	namespace         string                    // The root directory in etcd for services.
	client            client.KeysAPI            // The etcd client.
	mux               *Mux                      // The serve mux to keep in sync with etcd.
	waitIndex         uint64                    // Wait index to use when watching etcd.
	services          map[string]*ServiceRecord // Currently connected services.
	serviceNameRoutes map[string]string
}

// NewExchange creates a new exchange configured to watch for changes in a
// given etcd directory.
func NewExchange(namespace string, client client.KeysAPI, mux *Mux) *Exchange {
	return &Exchange{
		namespace:         namespace,
		client:            client,
		mux:               mux,
		services:          make(map[string]*ServiceRecord),
		serviceNameRoutes: make(map[string]string)}
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
	services, err := exchange.client.Get(ctx, exchange.namespace, options)
	if err != nil {
		CheckEtcdErrors(err)
	}

	for _, service := range services.Node.Nodes {
		for _, environ := range service.Nodes {
			if EnvMatch(environ.Key) {
				log.Printf("\n>\tMatched Environment: %v", environ.Key)
				var serviceRecord *ServiceRecord
				var serviceMachines []*Machine
				for _, config := range environ.Nodes {
					if strings.Compare(Tail(config.Key), "routes") == 0 {
						log.Printf("\n>\tMatched Routes: %v", config.Key)
						serviceRecord = exchange.load(config.Value, Name(service.Key))
					} else if strings.Compare(Tail(config.Key), "hosts") == 0 {
						log.Printf("\n>\tMatched Hosts: %v", config.Key)
						for _, host := range config.Nodes {
							if strings.Compare(host.Value, "") != 0 {
								serviceMachines = append(serviceMachines, &Machine{host.Key, host.Value})
							}
						}
					}
				}
				for _, machine := range serviceMachines {
					serviceRecord.ID = machine.ID
					serviceRecord.Address = machine.IP
					serviceRecord.Name = Name(service.Key)
					exchange.Register(serviceRecord)
				}
			}
		}
	}

	// We want to watch changes *after* this one.
	exchange.waitIndex = services.Index + 1

	return nil
}

type Machine struct {
	ID, IP string
}

func Name(s string) string {
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	split := strings.Split(s, "/")
	return split[1]
}

func Tail(s string) string {
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	split := strings.Split(s, "/")
	return split[len(split)-1]
}

func TailMinusOne(s string) string {
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	split := strings.Split(s, "/")
	return split[len(split)-2]
}

func EnvMatch(s string) bool {
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	split := strings.Split(s, "/")
	if strings.Compare(split[2], os.Getenv("VINE_ENV")) == 0 {
		return true
	}
	return false
}

func EnvKey(s string) string {
	return strings.Join(strings.Split(strings.TrimPrefix(strings.TrimSuffix(s, "/"), "/"), "/")[:3], "/")
}

// Watch observes changes in etcd and registers and unregisters services, as
// necessary, with the ExchangeServeMux.  This blocking call will terminate
// when a value is received on the stop channel.
func (exchange *Exchange) Watch() {
	ns := Namespace()
	opts := EtcdWatcherOptions(exchange.waitIndex)
	watcher := exchange.client.Watcher(ns, opts)
	for true {
		response, err := watcher.Next(context.TODO())
		if response.Node != nil {
			log.Printf("\n>\tRESPONDING TO:%v\n>\tFOR KEY:%v\n", response.Action, response.Node.Key)
		} else if response.PrevNode != nil {
			log.Printf("\n>\tRESPONDING TO:%v\n>\tFOR KEY:%v\n", response.Action, response.PrevNode.Key)
		} else {
			log.Printf("\n>\tRESPONDING TO:%v\n", response.Action)
		}
		if err != nil {
			log.Printf("\n>RECEIVED ERROR RESPONDING TO:%v\n>\tFOR KEY:%v\n>\tERROR: %+v", response.Action, response.PrevNode.Key, err)
			continue
		}

		switch response.Action {
		case "set", "update", "create", "compareAndSwap":
			if EnvMatch(response.Node.Key) {
				log.Printf("\n>\tMatched Environment: %v", response.Node.Key)
				if strings.Compare("routes", Tail(response.Node.Key)) == 0 {
					resp, err := exchange.client.Get(context.TODO(), EnvKey(response.Node.Key), EtcdGetOptions())
					CheckEtcdErrors(err)
					environ := resp.Node
					var serviceRecord *ServiceRecord
					var serviceMachines []*Machine
					for _, config := range environ.Nodes {
						if strings.Compare(Tail(config.Key), "routes") == 0 {
							log.Printf("\n>\tMatched Routes: %v", config.Key)
							serviceRecord = exchange.load(config.Value, Name(response.Node.Key))
						} else if strings.Compare(Tail(config.Key), "hosts") == 0 {
							log.Printf("\n>\tMatched Hosts: %v", config.Key)
							for _, host := range config.Nodes {
								if strings.Compare(host.Value, "") != 0 {
									serviceMachines = append(serviceMachines, &Machine{Tail(host.Key), host.Value})
								}
							}
						}
					}
					for _, machine := range serviceMachines {
						serviceRecord.ID = machine.ID
						serviceRecord.Address = machine.IP
						serviceRecord.Name = Name(response.Node.Key)
						exchange.Register(serviceRecord)
					}
				}
			} else if strings.Compare("hosts", TailMinusOne(response.Node.Key)) == 0 {
				name := Name(response.Node.Key)
				if serviceRoutes, ok := exchange.serviceNameRoutes[name]; ok {
					serviceRecord := exchange.load(serviceRoutes, name)
					serviceRecord.ID = Tail(response.Node.Key)
					serviceRecord.Address = response.Node.Value
					serviceRecord.Name = name
					exchange.Register(serviceRecord)
				} else {
					resp, err := exchange.client.Get(context.TODO(), EnvKey(response.Node.Key), EtcdGetOptions())
					CheckEtcdErrors(err)
					environ := resp.Node
					var serviceRecord *ServiceRecord
					var serviceMachines []*Machine
					for _, config := range environ.Nodes {
						if strings.Compare(Tail(config.Key), "routes") == 0 {
							log.Printf("\n>\tMatched Routes: %v", config.Key)
							serviceRecord = exchange.load(config.Value, Name(response.Node.Key))
						} else if strings.Compare(Tail(config.Key), "hosts") == 0 {
							log.Printf("\n>\tMatched Hosts: %v", config.Key)
							for _, host := range config.Nodes {
								if strings.Compare(host.Value, "") != 0 {
									serviceMachines = append(serviceMachines, &Machine{Tail(host.Key), host.Value})
								}
							}
						}
					}
					for _, machine := range serviceMachines {
						serviceRecord.ID = machine.ID
						serviceRecord.Address = machine.IP
						serviceRecord.Name = Name(response.Node.Key)
						exchange.Register(serviceRecord)
					}
				}
			}
		case "delete", "expire", "compareAndDelete":
			if EnvMatch(response.Node.Key) {
				if strings.Compare("routes", Tail(response.Node.Key)) == 0 {
					resp, err := exchange.client.Get(context.TODO(), EnvKey(response.Node.Key), EtcdGetOptions())
					CheckEtcdErrors(err)
					environ := resp.Node
					var serviceMachines []*Machine
					for _, config := range environ.Nodes {
						if strings.Compare(config.Key, "hosts") == 0 {
							for _, host := range config.Nodes {
								if strings.Compare(host.Value, "") != 0 {
									serviceMachines = append(serviceMachines, &Machine{Tail(host.Key), host.Value})
								}
							}
						}
					}
					for _, machine := range serviceMachines {
						if service, ok := exchange.services[machine.ID]; ok {
							exchange.Unregister(service)
						}
					}
					// Eliminate old service machines with same name
					for _, service := range exchange.services {
						if strings.Compare(service.Name, Name(response.Node.Key)) == 0 {
							exchange.Unregister(service)
						}
					}
				} else if strings.Compare("hosts", TailMinusOne(response.Node.Key)) == 0 {
					if service, ok := exchange.services[Tail(response.Node.Value)]; ok {
						exchange.Unregister(service)
					}
				}
			}
		}
		go func(exchange *Exchange) {
			for _, method := range []string{"GET", "PUT", "POST", "DELETE", "PATCH"} {
				if arr, ok := exchange.mux.routes[method]; ok {
					for _, handler := range arr {
						log.Printf("\n>\tHANDLER: %+v\n", handler)
					}
					log.Printf("\n>\tNUMBER OF CURRENTLY REGISTERED %v PATTERNS: %v\n", method, len(arr))
				}
			}
			address, key := gatewayNamespace()
			opts := gatewaySetOpts()
			resp, err := exchange.client.Set(context.Background(), key, address, opts)
			if err != nil {
				log.Println("ERROR: ", err)
			} else {
				//log.Printf("\n>\t%v \"%v\"\n>\t%v%v", pInfoInline("Success Gateway Alive At:"), pInfoInline(address), pInfoInline("Services May Locate This Gateway At The Key Provided Below\n>\tGATEWAY_KEY="), pInfoInline(resp.Node.Key))
			}

		}(exchange)
	}
}

func getEnvironmentKey(s string) {

}

// func getRootNode(key string) string {
//
// 	rootkey := ""
// 	splitKeys := strings.Split(key, "/")
// 	for i, v := range splitKeys {
// 		if i < 3 {
// 			rootkey += v + "/"
// 		}
// 	}
// 	return rootkey
// }

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
		uName = "/gateway/environments/" + os.Getenv("VINE_ENV") + "/" + string(outputUName)
		log.Printf("\n>\tUNAME:\n>%v\n>\tHOST ADDRESS:\n>\t%v", uName, host)
	} else {
		host = "127.0.0.1"
		uName = "/gateway/environments/" + os.Getenv("VINE_ENV") + "/" + string(outputUName)
		log.Printf("\n>\tUNAME:\n>%v\n>\tHOST ADDRESS:\n>\t%v", uName, host)
	}
	return strings.Join([]string{host, ":", os.Getenv("PORT")}, ""), uName
}

func gatewaySetOpts() *client.SetOptions {
	opts := &client.SetOptions{}
	opts.Refresh = true
	opts.TTL = time.Second * 60
	return opts
}

// func (exchange *Exchange) PublishLocation() {
// 	go func(exchange *Exchange) {
// 		for {
// 			time.Sleep(time.Second * 30)
// 			for _, method := range []string{"GET", "PUT", "POST", "DELETE", "PATCH"} {
// 				if arr, ok := exchange.mux.routes[method]; ok {
// 					// for _, handler := range arr {
// 					// 	log.Printf("\n>\tHANDLER: %+v\n", handler)
// 					// }
// 					log.Printf("\n>\tNUMBER OF CURRENTLY REGISTERED %v PATTERNS: %v\n", method, len(arr))
// 				}
// 			}
// 			address, key := gatewayNamespace()
// 			opts := gatewaySetOpts()
// 			resp, err := exchange.client.Set(context.Background(), key, address, opts)
// 			if err != nil {
// 				log.Println("ERROR: ", err)
// 			} else {
// 				log.Printf("\n>\t%v \"%v\"\n>\t%v%v", pInfoInline("Success Gateway Alive At:"), pInfoInline(address), pInfoInline("Services May Locate This Gateway At The Key Provided Below\n>\tGATEWAY_KEY="), pInfoInline(resp.Node.Key))
// 			}
// 		}
// 	}(exchange)
// }

// func unregisterNodes(exchange *Exchange, node *client.Node) {
// 	for _, n := range node.Nodes {
// 		unregisterNode(exchange, n)
// 	}
// }
//
// func unregisterNode(exchange *Exchange, n *client.Node) {
// 	pInfo("\n>\tUnregistering Key: %v\n", n.Key)
// 	if MatchHostsEnv(n.Key) {
// 		defer func() {
// 			if perr := recover(); perr != nil {
// 				var ok bool
// 				perr, ok = perr.(error)
// 				if !ok {
// 					fmt.Errorf("Panicking: %v", perr)
// 				}
// 			}
// 		}()
// 		if service, ok := exchange.services[ID(n.Key)]; ok {
// 			service.Address = n.Value
// 			exchange.Unregister(service)
// 		}
//
// 	} else if MatchEnv(n.Key) {
// 		defer func() {
// 			if perr := recover(); perr != nil {
// 				var ok bool
// 				perr, ok = perr.(error)
// 				if !ok {
// 					fmt.Errorf("Panicking: %v", perr)
// 				}
// 			}
// 		}()
// 		if service, ok := exchange.services[ID(n.Key)]; ok {
// 			host := Host(n.Key)
// 			resp, err := exchange.client.Get(context.Background(), host, EtcdGetDirectOptions())
// 			CheckEtcdErrors(err)
// 			for _, respNode := range resp.Node.Nodes {
// 				go func(exchange *Exchange, respNode *client.Node, service *ServiceRecord) {
// 					service.Address = respNode.Value
// 					exchange.Unregister(service)
// 				}(exchange, respNode, service)
// 			}
// 		}
// 	}
// }

func (exchange *Exchange) load(js, name string) *ServiceRecord {
	var routes []EtcdRoute
	var s ServiceRecord
	json.Unmarshal(bytes.NewBufferString(js).Bytes(), &routes)
	s.GenerateRecord(routes)
	exchange.serviceNameRoutes[name] = js
	return &s
}

// Register adds routes exposed by a service to the ExchangeServeMux.
func (exchange *Exchange) Register(service *ServiceRecord) {
	exchange.services[service.ID] = service
	for method, patterns := range service.Routes {
		for _, pattern := range patterns {
			pattern = "/api" + pattern
			log.Printf("\n>\tADDING PATTERN\n>\tPATTERN DETAILS: %v %v\n>\tSERVICE DETAILS: %v %v", method, pattern, service.Address, service.ID)
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
