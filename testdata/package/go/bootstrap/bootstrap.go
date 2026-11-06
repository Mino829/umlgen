package bootstrap

// Imports-only dependencies survive even though this file has no types.
import (
	_ "example.test/packagefixture/service"
	_ "example.test/telemetry"
)

func Start() {}
