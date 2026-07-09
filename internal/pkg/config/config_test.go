package config

import (
	"flag"
	"testing"
)

func TestRegisterFlagsParsesMockInventoryFile(t *testing.T) {
	configFile = ""
	mockInventoryFile = ""

	fs := flag.NewFlagSet("mock-config", flag.ContinueOnError)
	RegisterFlags(fs)

	if err := fs.Parse([]string{
		"--device-config-file=/tmp/device-config.yaml",
		"--mock-inventory-file=/tmp/mock-inventory.yaml",
	}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if configFile != "/tmp/device-config.yaml" {
		t.Fatalf("expected configFile to be parsed, got %q", configFile)
	}
	if mockInventoryFile != "/tmp/mock-inventory.yaml" {
		t.Fatalf("expected mockInventoryFile to be parsed, got %q", mockInventoryFile)
	}
}
