package moria

import (
	"log"
	"os"

	"github.com/coreos/etcd/client"
)

// Configure Generates an Etcd api client for key retrieval
func Configure(e string) *Exchange {
	Initialize()
	urls := EtcdURL()
	cfg := EtcdConfig(urls)
	c, err := New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	etcd := client.NewKeysAPI(c)
	mux := NewMux()
	namespace := Namespace()
	exchange := NewExchange(namespace, etcd, mux)
	exchange.Init()

	// Watch for service changes in etcd.  The exchange updates service
	// routing rules based on configuration changes in etcd.
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("\n\n\n\n\n\n\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\tMAJOR LIFE THREATENING ERROR\n>\t%+v", err)
				exchange.Watch()
			}
		}()
		log.Print("Watching for service configuration changes in etcd")
		exchange.Watch()
	}()
	return exchange
}

// Namespace sets a custom etcd namespace key or uses the default `services` key
func Namespace() string {
	ns := os.Getenv("NAMESPACE")
	if ns != "" {
		return ns
	}
	return "services"
}
