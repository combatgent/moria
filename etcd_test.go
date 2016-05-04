package moria_test

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/combatgent/moria"
	etcd "github.com/coreos/etcd/client"
	"golang.org/x/net/context"
)

func TestNew(t *testing.T) {
	urlString := "http://localhost:8000,http://localhost:8001,http://localhost:8002"
	os.Setenv("ETCD_USERNAME", "USERNAME")
	os.Setenv("ETCD_PASSWORD", "PASSWORD")

	os.Setenv("ETCD_CA_STRING", "-----BEGIN CERTIFICATE-----\nMIIDAzCCAeugAwIBAgIJAPNHXtZph49uMA0GCSqGSIb3DQEBBQUAMBgxFjAUBgNV\nBAMMDXN1cGVyZmFrZS5jb20wHhcNMTYwNDI5MTgzOTQ3WhcNMjYwNDI3MTgzOTQ3\nWjAYMRYwFAYDVQQDDA1zdXBlcmZha2UuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOC\nAQ8AMIIBCgKCAQEA6HwulxtEcbXZuFUBFtQ0jMboaEmUbXadO6cR2CzogkdgJ/s+\nCG7yVQMmTCGxAh3BTY39//TncD7Tioa/1HK84tUOPqgSPYlulWnZPdeUr2L67x/X\nypFv4A1EZTUZfUihocWnFTIyV3S4iOXKk41eHtt0O+aaP3uAa0/c/bmEKt8LoGAM\nrsDSNSv7l+f1xe3YBYZqMry4PO4R1j4uiaxfFbWEdRAtciE0oiuXEXBTzw2+L4iH\nLqOa4vKVbtgi5lGCSDUo2mLehvlMtuHb3ltFpgkOIh6QtJJy5/FWZu2BVSMweGmN\nfi/2994ZtEzedD5CBfdyB2oqOsM69OQrZVl1/QIDAQABo1AwTjAdBgNVHQ4EFgQU\niIhDO7D/a2lk/Xu+S8tjX108DPkwHwYDVR0jBBgwFoAUiIhDO7D/a2lk/Xu+S8tj\nX108DPkwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQUFAAOCAQEARU7zh1cpXEUD\nMlQmMwzsvKjk3okTEY0qZGGi3XX3HwhTXaMYyNYgcgTVjSpjS0qfJRja2bt+nnxw\nJqayI9/IrgdQgCOcRBPqt8x6RfPaExJSVBHIYI1kU1p+wsJE6JJcJOd9EN15F43y\nAFdYu2uN2/Y9u9+rQvNjMfwH/RbLbqPDNL34BKWuZiA1VVat8qLNyIKyCE0yuiOI\nrif1+SdLB9yjeagyiqflhNHWVO4VHfDS5BcU/SjM7oGj88PIHju9dx20OECra4ZG\nkipnEaSnzYlkewut1IiyZ12aLlg5wTUrz9BuojB8A2ArGueZ9LFS9D/Q+rOaUvx+\nQbCru6jejg==\n-----END CERTIFICATE-----\n")
	os.Setenv("ETCD_CLIENT_PEERS", urlString)
	urls := moria.EtcdURL()
	cfg := moria.EtcdConfig(urls)
	testClient, err := moria.New(cfg)
	if err != nil {
		t.Errorf("Unexpected error got %+v", err)
	}
	if reflect.TypeOf(testClient).AssignableTo(reflect.TypeOf((*etcd.Client)(nil))) {
		t.Errorf("Expected type %+v got %+v", reflect.TypeOf(testClient), reflect.TypeOf((*etcd.Client)(nil)))
	}
}

func TestEtcdURL(t *testing.T) {
	os.Setenv("ETCD_CLIENT_PEERS", "http://localhost:8000,http://localhost:8001,http://localhost:8002")
	urls := moria.EtcdURL()
	if len(urls) != 3 {
		t.Errorf("EtcdURL should be a comma separated list of length 3")
	}
}

func TestEtcdConfig(t *testing.T) {
	urlString := "http://localhost:8000,http://localhost:8001,http://localhost:8002"
	os.Setenv("ETCD_USERNAME", "USERNAME")
	os.Setenv("ETCD_PASSWORD", "PASSWORD")

	os.Setenv("ETCD_CA_STRING", "-----BEGIN CERTIFICATE-----\nMIIDAzCCAeugAwIBAgIJAPNHXtZph49uMA0GCSqGSIb3DQEBBQUAMBgxFjAUBgNV\nBAMMDXN1cGVyZmFrZS5jb20wHhcNMTYwNDI5MTgzOTQ3WhcNMjYwNDI3MTgzOTQ3\nWjAYMRYwFAYDVQQDDA1zdXBlcmZha2UuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOC\nAQ8AMIIBCgKCAQEA6HwulxtEcbXZuFUBFtQ0jMboaEmUbXadO6cR2CzogkdgJ/s+\nCG7yVQMmTCGxAh3BTY39//TncD7Tioa/1HK84tUOPqgSPYlulWnZPdeUr2L67x/X\nypFv4A1EZTUZfUihocWnFTIyV3S4iOXKk41eHtt0O+aaP3uAa0/c/bmEKt8LoGAM\nrsDSNSv7l+f1xe3YBYZqMry4PO4R1j4uiaxfFbWEdRAtciE0oiuXEXBTzw2+L4iH\nLqOa4vKVbtgi5lGCSDUo2mLehvlMtuHb3ltFpgkOIh6QtJJy5/FWZu2BVSMweGmN\nfi/2994ZtEzedD5CBfdyB2oqOsM69OQrZVl1/QIDAQABo1AwTjAdBgNVHQ4EFgQU\niIhDO7D/a2lk/Xu+S8tjX108DPkwHwYDVR0jBBgwFoAUiIhDO7D/a2lk/Xu+S8tj\nX108DPkwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQUFAAOCAQEARU7zh1cpXEUD\nMlQmMwzsvKjk3okTEY0qZGGi3XX3HwhTXaMYyNYgcgTVjSpjS0qfJRja2bt+nnxw\nJqayI9/IrgdQgCOcRBPqt8x6RfPaExJSVBHIYI1kU1p+wsJE6JJcJOd9EN15F43y\nAFdYu2uN2/Y9u9+rQvNjMfwH/RbLbqPDNL34BKWuZiA1VVat8qLNyIKyCE0yuiOI\nrif1+SdLB9yjeagyiqflhNHWVO4VHfDS5BcU/SjM7oGj88PIHju9dx20OECra4ZG\nkipnEaSnzYlkewut1IiyZ12aLlg5wTUrz9BuojB8A2ArGueZ9LFS9D/Q+rOaUvx+\nQbCru6jejg==\n-----END CERTIFICATE-----\n")
	os.Setenv("ETCD_CLIENT_PEERS", urlString)
	urls := moria.EtcdURL()
	cfg := moria.EtcdConfig(urls)
	if cfg.Username != "USERNAME" ||
		cfg.Password != "PASSWORD" ||
		len(cfg.Endpoints) != len(strings.Split(urlString, ",")) ||
		cfg.Transport == nil {
		t.Errorf("Client config failed or was imporperly configured\nExpected USERNAME got %v\nExpected PASSWORD got %v\nExpected%v got %v", cfg.Username, cfg.Password, len(strings.Split(urlString, ",")), len(cfg.Endpoints))
	}
}

func TestEtcdConfigPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	urlString := "http://localhost:8000,http://localhost:8001,http://localhost:8002"
	os.Setenv("ETCD_USERNAME", "USERNAME")
	os.Setenv("ETCD_PASSWORD", "PASSWORD")

	os.Setenv("ETCD_CA_STRING", "-----BEGIN CERTIFICATE-----\nMIIDAzCCAeugAIb3DQEBBQUAMBgxFjAUBgNV\nBAMMDXN1cGVyZmFrZS5jb20wHhcNMTYwNDI5MTgzOTQ3WhcNMjYwNDI3MTgzOTQ3\nWjAYMRYwFAYDVQQDDA1zdXBlcmZha2UuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOC\nAQ8AMIIBCgKCAQEA6HwulxtEcbXZuFUBFtQ0jMboaEmUbXadO6cR2CzogkdgJ/s+\nCG7yVQMmTCGxAh3BTY39//TncD7Tioa/1HK84tUOPqgSPYlulWnZPdeUr2L67x/X\nypFv4A1EZTUZfUihocWnFTIyV3S4iOXKk41eHtt0O+aaP3uAa0/c/bmEKt8LoGAM\nrsDSNSv7l+f1xe3YBYZqMry4PO4R1j4uiaxfFbWEdRAtciE0oiuXEXBTzw2+L4iH\nLqOa4vKVbtgi5lGCSDUo2mLehvlMtuHb3ltFpgkOIh6QtJJy5/FWZu2BVSMweGmN\nfi/2994ZtEzedD5CBfdyB2oqOsM69OQrZVl1/QIDAQABo1AwTjAdBgNVHQ4EFgQU\niIhDO7D/a2lk/Xu+S8tjX108DPkwHwYDVR0jBBgwFoAUiIhDO7D/a2lk/Xu+S8tj\nX108DPkwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQUFAAOCAQEARU7zh1cpXEUD\nMlQmMwzsvKjk3okTEY0qZGGi3XX3HwhTXaMYyNYgcgTVjSpjS0qfJRja2bt+nnxw\nJqayI9/IrgdQgCOcRBPqt8x6RfPaExJSVBHIYI1kU1p+wsJE6JJcJOd9EN15F43y\nAFdYu2uN2/Y9u9+rQvNjMfwH/RbLbqPDNL34BKWuZiA1VVat8qLNyIKyCE0yuiOI\nrif1+SdLB9yjeagyiqflhNHWVO4VHfDS5BcU/SjM7oGj88PIHju9dx20OECra4ZG\nkipnEaSnzYlkewut1IiyZ12aLlg5wTUrz9BuojB8A2ArGueZ9LFS9D/Q+rOaUvx+\nQbCru6jejg==\n-----END CERTIFICATE-----\n")
	os.Setenv("ETCD_CLIENT_PEERS", urlString)
	urls := moria.EtcdURL()
	cfg := moria.EtcdConfig(urls)
	if cfg.Username != "USERNAME" ||
		cfg.Password != "PASSWORD" ||
		len(cfg.Endpoints) != len(strings.Split(urlString, ",")) ||
		cfg.Transport == nil {
		t.Errorf("Client config failed or was imporperly configured\nExpected  cfg.Username == \"USERNAME\" got %v\n Expected cfg.Password == \"PASSWORD\" got %v\nExpected len(cfg.Endpoints) == len(urlString) got %v and %v\nExpected cfg.Transport != nil got %v\n", cfg.Username, cfg.Password, len(cfg.Endpoints), len(strings.Split(urlString, ",")), cfg.Transport)
	}
}

func TestEtcdGetOptions(t *testing.T) {
	opt := moria.EtcdGetOptions()
	if opt.Recursive != true ||
		opt.Sort != false ||
		opt.Quorum != false {
		t.Error("EtcdGet options failed or imporperly configured")
	}
}

func TestEtcdGetDirectOptions(t *testing.T) {
	opt := moria.EtcdGetDirectOptions()
	if opt.Recursive != false ||
		opt.Sort != false ||
		opt.Quorum != false {
		t.Error("EtcdGet options failed or imporperly configured")
	}
}

// EtcdSetOptions returns an options struct for the etcd set keys request
func TestEtcdSetOptions(t *testing.T) {
	opt := moria.EtcdSetOptions()
	if opt.PrevIndex != 0 ||
		opt.PrevExist != etcd.PrevNoExist ||
		opt.Refresh != false ||
		opt.Dir != false {
		t.Error("EtcdSet options failed or imporperly configured")
	}
}

// EtcdWatcherOptions returns an options struct for the etcd Watcher object
func TestEtcdWatcherOptions(t *testing.T) {
	var index uint64
	index = 9
	opt := moria.EtcdWatcherOptions(index)
	if opt.AfterIndex != index ||
		opt.Recursive != true {
		t.Error("EtcdWatcherOptions failed or imporperly configured")
	}
}

func TestMatchEnvTrue(t *testing.T) {
	os.Setenv("VINE_ENV", "test")
	s := "/services/tower-products-api/test/routes"
	valid := moria.MatchEnv(s)
	if !valid {
		t.Error("EtcdWatcherOptions ")
	}
}

func TestMatchEnvFalseNotEnv(t *testing.T) {
	os.Setenv("VINE_ENV", "test")
	s := "/services/tower-products-api/not-test/routes"
	valid := moria.MatchEnv(s)
	if valid {
		t.Error("EtcdWatcherOptions ")
	}
}

func TestMatchEnvFalseNotRoutes(t *testing.T) {
	os.Setenv("VINE_ENV", "test")
	s := "/services/tower-products-api/test/not-routes"
	valid := moria.MatchEnv(s)
	if valid {
		t.Error("EtcdWatcherOptions ")
	}
}

func TestID(t *testing.T) {
	serviceName := "tower-products-api"
	id := moria.ID("/services/tower-products-api/test/routes")
	if id != serviceName {
		t.Error("Service ID was not properly returned")
	}
}
func TestIDBadKey(t *testing.T) {
	serviceName := ""
	id := moria.ID("/services")
	if id != serviceName {
		t.Error("Service ID was not properly returned")
	}
}

func TestHost(t *testing.T) {
	key := "/services/tower-products-api/test/routes"
	h := moria.Host(key)
	if h != "services/tower-products-api/test/hosts" {
		t.Errorf("Hosts key was not properly returned\nExpected %v got %v", "/services/tower-products-api/test/hosts", h)
	}
}

func TestCheckEtcdErrorsContextCanceled(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	err := context.Canceled
	moria.CheckEtcdErrors(err)
}

func TestCheckEtcdErrorsDeadlineExceeded(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	err := context.DeadlineExceeded
	moria.CheckEtcdErrors(err)
}

func TestCheckEtcdErrorsClusterError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	err := new(etcd.ClusterError)
	moria.CheckEtcdErrors(err)
}

func TestCheckEtcdErrorsBadEndpoints(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	err := errors.New("Bad endpoints")
	moria.CheckEtcdErrors(err)
}
