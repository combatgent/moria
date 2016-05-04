package moria_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/combatgent/moria"
)

func TestMain(m *testing.M) {
	_ = createEnv()
	code := m.Run()
	pattern := ".env*"
	matches, _ := filepath.Glob(pattern)
	for _, match := range matches {
		os.Remove(match)
	}
	os.Exit(code)
}

func createEnv() *os.File {
	tmpfile, _ := ioutil.TempFile(".", ".env")
	ioutil.WriteFile(".env", []byte("# TESTENV\n\nVINE_ENV=test\nETCD_USERNAME=root\nNAMEPACE=services\nETCD_PASSWORD=YDAKJCPXAMXSNFXW\nETCD_CA_PATH=~/some/path/to/key\nETCD_CA_CERT_PATH=~/some/path/to/key\nETCD_CLIENT_PEERS=https://test.com:10682,https://test.com:10682,https://test.com:27144\nPORT=8090\nETCD_CA_STRING=-----BEGIN CERTIFICATE-----\\nMIMGIxM2M3ZDlCAmOgAwIBAgIEVsudczANBgkqhkiG9w0BAQ0FADA/IDezCzYjk3Njc4MT0wOwYDVQQDDDRD\\nb21iYXRhbnQgR2VudGxlbWVuLWQlMWMyYzRhMjY4OWYx\\nZjY4MB4XDTE2MDIyMjIzNDQ1MVoXDTE3MDIyMTIzNDQ1MVowPzE9MDsGA1UEAww0\\nQ29tYmF0YW50IEdlbnRsZW1lbi1kM2I5NzY3ODBiMTNjN2Q5ZTFjMmM0YTI2ODlm\\nMWY2ODCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAMpANi6GkBngBq/n\\nuLL+yjJdY/h/mEP7vPXJQVFFv8i020TMsG7KIe68xKIstAZ7QCVrZGrbg1FRL4plcMSAH1+7CoTiqUB/EsKykkFp0+MkSv8hGf2GI2njPyHsdF7etDa9Wtk\\nrnhB7++je\\ncufyZzDRuZjB8TCQ2Vz2QrYzibHj0tk7ylCdDoZJx5HMZWoDF369+0TdFlfx0HSg\\nmWxvxn+OGPbFVJqCk6ZGtPdID+KUi/8NjS+mVSiFOrujQHy+uzLf3zJGuODTpQ77\\nReasRruQXq8nBGDX+aUEPPTiHmdU1/dLDpx4rD2XKpUqvupgkak2aWaR3oXRO8X/\\nLEj5SzECAwEAAaN/MH0wHQYDVR0OBBYEFP9D8vwLkkvY5Y9qr9WwjsRp3NLjMA4G\\nA1UdDwEB/wQEAwICBDAdBgNVHSUEFjAUBggrBgEFBQcDAQYIKwYBBQUHAwIwDAYD\\nVR0TBAUwAwEB/zAfBgNVHSMEGDAWgBT/Q/L8C5JL2OWPaq/VsI7EadzS4zANBgkq\\nhkiG9w0BAQ0FAAOCAQEAFUPd8jv6viHLujl1f1FUoz64deTDbLGWpnT6aJXRWFaF\\nBM4wYIrReuLdzjK8eeprNiY+t0wAbUQTTwLHCPk+R4xK1h54wCYuTZMV3YVS7DK8\\nVCU5ASUMwvdWv7qCiD8YwMT8aqD4a7UOg2g5WGawCDH0xlN4wZdPlrjjC65CvD6K\\nOUth+jdcgR/2rS9+32J+aVitM1ggh0gK8m64HHSgnXBRLDrft5qvVWankcmnb+t+\\n6/pPlgeK6TkhKGvNhQ+T66cWE11gFADfrPHarhTuO92FzgcLNi9j+IIiMk0GnHGx\\nbqridEjk6JKd3q5uvuPyhG6Ig2QzUoXU/tY941MdqQ==\\n-----END CERTIFICATE-----\\n"), 0777)
	return tmpfile
}

func TestEnvironment(t *testing.T) {
	env := moria.Environment()
	if env != "test" {
		t.Errorf("Expected env to be \"test\" got %v", env)
	}
}

// func TestEnvironmentFailMissingRequired(t *testing.T) {
//
// 	defer func() {
//
// 		if r := recover(); r == nil {
// 			t.Errorf("The code did not panic")
// 		}
// 	}()
// 	os.Clearenv()
// 	os.Setenv("ETCD_CA_STRING", "")
// 	_ = moria.Environment()
//
// }

func TestEnvironmentInvalidAuxEnv(t *testing.T) {

	defer func() {

		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	os.Setenv("ENV_PATH", "alskdjflkjdfxlkjhnoweuhrfoweuvb")
	_ = moria.Environment()
}

func TestEnvironmentMissingGoEnv(t *testing.T) {

	defer func() {

		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	os.Setenv("VINE_ENV", "")
	_ = moria.Environment()
}
