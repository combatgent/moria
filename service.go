package moria

import (
	"strings"
)

// Routes maps HTTP methods to URLs.
type Routes map[string][]string

// ServiceRecord is a representation of a service stored in etcd and used by
// exchanges.
type ServiceRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
	Routes  Routes `json:"routes"`
}

// GenerateRecord Creates a service record for the grape etcd path export
func (s *ServiceRecord) GenerateRecord(routes []EtcdRoute) {
	s.Routes = make(Routes, 0)
	for _, r := range routes {
		routesArray, present := s.Routes[r.Method]
		if !present {
			routesArray = make([]string, 0)
			s.Routes[r.Method] = routesArray
		}
		s.Routes[r.Method] = append(s.Routes[r.Method], strings.Replace(r.Path, "(.:format)", "", -1))
	}
}
