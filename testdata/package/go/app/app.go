package app

import model "example.test/packagefixture/domain"

// Missing deliberately has no declaration.
type App struct {
	Item    model.Item
	Unknown Missing
}
