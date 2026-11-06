package domain

import "example.test/packagefixture/service"

// The domain/service import cycle is intentional: this is a syntax-only fixture.
type Item struct {
	Service *service.Service
}
