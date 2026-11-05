package checkout

import (
	"example.test/checkout/pricing"
	"example.test/checkout/store"
)

type Repository interface {
	Save(int)
}

type LocalRepository struct{}

func (LocalRepository) Save(amount int) { store.Stored() }

func Input() int  { return 1 }
func Check() bool { return true }
func Cleanup()    { store.Stored() }
func Worker()     { store.Stored() }

func PlaceOrder(valid bool, repo Repository, notify func()) {
	amount := pricing.Total(Input())
	if valid && Check() {
		repo.Save(amount)
	} else {
		store.Reject()
	}
	for n := 0; n < 2; n++ {
		store.Audit(n)
	}
	defer Cleanup()
	go Worker()
	notify()
}

func Exchange(ch chan int) {
	ch <- Input()
	value := <-ch
	_ = value
	select {
	case <-ch:
		Cleanup()
	default:
		Worker()
	}
}

func Recur() { Recur() }

func Closures() {
	saved := func() { store.Stored() }
	_ = saved
	(func() { store.Audit(Input()) })()
}
