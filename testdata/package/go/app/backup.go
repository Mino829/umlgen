package app

import model "example.test/packagefixture/alternate"

// The same alias resolves differently in this file.
type Backup struct {
	Item model.Item
}
