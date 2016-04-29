package moria_test

import (
	"testing"

	"github.com/combatgent/moria"
	// env "github.com/joho/godotenv"
)

func TestInitialize(t *testing.T) {
	// env.Load(".env_test")
	var err error
	moria.Initialize()
	if err != nil {
		t.Errorf("Error logging", err)
	}
}
