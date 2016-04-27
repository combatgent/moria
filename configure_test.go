package moria_test

import (
	"os"
	"testing"

	"github.com/combatgent/moria"
	// env "github.com/joho/godotenv"
)

func TestNamespace(t *testing.T) {
	// env.Load(".env_test")
	ns := moria.Namespace()
	if ns != "services" {
		t.Errorf("Expected \"services\" got %+v", ns)
	}
	os.Setenv("NAMESPACE", "TEST")
	ns = moria.Namespace()
	if ns != "TEST" {
		t.Errorf("Expected \"TEST\" got %+v", ns)
	}
}
