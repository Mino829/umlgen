package watch

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"testing"
	"testing/synctest"
	"time"
)

type watchConsumer struct {
	snapshot     Snapshot
	observeError error
	generate     func(context.Context) error
	generations  []Snapshot
	reports      []error
	cancel       context.CancelFunc
	result       chan error
}

func startConsumer(opts Options, snapshot Snapshot) *watchConsumer {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &watchConsumer{snapshot: snapshot, cancel: cancel, result: make(chan error, 1)}
	go func() {
		consumer.result <- Run(ctx, opts, func() (Snapshot, error) {
			return consumer.snapshot, consumer.observeError
		}, func(ctx context.Context) error {
			consumer.generations = append(consumer.generations, maps.Clone(consumer.snapshot))
			if consumer.generate != nil {
				return consumer.generate(ctx)
			}
			return nil
		}, func(err error) {
			consumer.reports = append(consumer.reports, err)
		})
	}()
	synctest.Wait()
	return consumer
}

func (consumer *watchConsumer) stop(t *testing.T) {
	t.Helper()
	consumer.cancel()
	synctest.Wait()
	if err := <-consumer.result; !errors.Is(err, context.Canceled) {
		t.Errorf("Run cancellation = %v, want context.Canceled", err)
	}
}

func advance(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

func digest(value byte) [32]byte {
	return [32]byte{value}
}

func TestRunComparesContentAndMembershipWithoutInitialGeneration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		consumer := startConsumer(Options{Interval: time.Second}, Snapshot{"/source.go": digest(1)})
		defer consumer.stop(t)
		advance(5 * time.Second)
		if len(consumer.generations) != 0 {
			t.Fatal("unchanged initial inputs generated output")
		}

		// Reuse the same borrowed map to ensure retained snapshots are isolated.
		consumer.snapshot["/source.go"] = digest(2)
		advance(time.Second)
		consumer.snapshot["/optional"] = [32]byte{}
		advance(time.Second)
		delete(consumer.snapshot, "/source.go")
		consumer.snapshot["/renamed.go"] = digest(2)
		advance(time.Second)
		delete(consumer.snapshot, "/optional")
		advance(time.Second)
		consumer.snapshot = Snapshot{}
		advance(time.Second)

		want := []Snapshot{
			{"/source.go": digest(2)},
			{"/source.go": digest(2), "/optional": {}},
			{"/renamed.go": digest(2), "/optional": {}},
			{"/renamed.go": digest(2)},
			{},
		}
		if !reflect.DeepEqual(consumer.generations, want) {
			t.Fatalf("generations = %#v, want %#v", consumer.generations, want)
		}
		advance(3 * time.Second)
		if len(consumer.generations) != len(want) || len(consumer.reports) != 0 {
			t.Fatalf("unchanged empty scope retried or reported: generations=%d reports=%v", len(consumer.generations), consumer.reports)
		}
	})
}

func TestRunDebouncesFromLastObservedChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		consumer := startConsumer(Options{Interval: 10 * time.Millisecond, Debounce: 55 * time.Millisecond}, Snapshot{"/source.go": digest(1)})
		defer consumer.stop(t)
		consumer.snapshot["/source.go"] = digest(2)
		advance(10 * time.Millisecond)
		advance(30 * time.Millisecond)
		consumer.snapshot["/source.go"] = digest(3)
		advance(10 * time.Millisecond)
		advance(54 * time.Millisecond)
		if len(consumer.generations) != 0 {
			t.Fatal("generated before the last change settled")
		}
		advance(time.Millisecond)
		if len(consumer.generations) != 1 || consumer.generations[0]["/source.go"] != digest(3) {
			t.Fatalf("settled generations = %#v", consumer.generations)
		}
		advance(time.Second)
		if len(consumer.generations) != 1 {
			t.Fatal("settled input generated more than once")
		}
	})
}

func TestRunCoalescesChangesDuringGeneration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		consumer := startConsumer(Options{Interval: 10 * time.Millisecond, Debounce: 20 * time.Millisecond}, Snapshot{"/source.go": digest(1)})
		defer consumer.stop(t)
		release := make(chan struct{})
		consumer.generate = func(ctx context.Context) error {
			if len(consumer.generations) == 1 {
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
				return errors.New("generation failed while inputs changed")
			}
			return nil
		}
		consumer.snapshot["/source.go"] = digest(2)
		advance(30 * time.Millisecond)
		if len(consumer.generations) != 1 {
			t.Fatalf("initial changed generation count = %d", len(consumer.generations))
		}
		consumer.snapshot["/source.go"] = digest(3)
		advance(time.Second)
		consumer.snapshot["/source.go"] = digest(4)
		advance(time.Second)
		if len(consumer.generations) != 1 {
			t.Fatal("generation overlapped a blocked predecessor")
		}
		close(release)
		synctest.Wait()
		advance(19 * time.Millisecond)
		if len(consumer.generations) != 1 {
			t.Fatal("busy changes did not receive a trailing debounce")
		}
		advance(time.Millisecond)
		if len(consumer.generations) != 2 || consumer.generations[1]["/source.go"] != digest(4) {
			t.Fatalf("busy changes were not coalesced into the newest input: %#v", consumer.generations)
		}
		if len(consumer.reports) != 1 {
			t.Fatalf("busy failure was not reported once: %v", consumer.reports)
		}
	})
}

func TestRunFailedGenerationNeedsNewInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		consumer := startConsumer(Options{Interval: time.Second}, Snapshot{"/source.go": digest(1)})
		defer consumer.stop(t)
		failure := errors.New("invalid source")
		consumer.generate = func(context.Context) error { return failure }
		consumer.snapshot["/source.go"] = digest(2)
		advance(time.Second)
		advance(10 * time.Second)
		if len(consumer.generations) != 1 || len(consumer.reports) != 1 || !errors.Is(consumer.reports[0], failure) {
			t.Fatalf("failed input retried or failure lost: generations=%d reports=%v", len(consumer.generations), consumer.reports)
		}
		consumer.observeError = errors.New("configuration temporarily unreadable")
		advance(2 * time.Second)
		consumer.observeError = nil
		advance(time.Second)
		if len(consumer.generations) != 1 || len(consumer.reports) != 2 {
			t.Fatalf("observation recovery retried failed input: generations=%#v reports=%v", consumer.generations, consumer.reports)
		}
		consumer.generate = nil
		consumer.snapshot["/source.go"] = digest(3)
		advance(time.Second)
		if len(consumer.generations) != 2 || consumer.generations[1]["/source.go"] != digest(3) || len(consumer.reports) != 2 {
			t.Fatalf("corrected input did not recover: generations=%#v reports=%v", consumer.generations, consumer.reports)
		}
	})
}

func TestRunUnpublishedGenerationWaitsForValidObservation(t *testing.T) {
	for _, cancelWhileInvalid := range []bool{false, true} {
		name := "recover"
		if cancelWhileInvalid {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				consumer := startConsumer(Options{Interval: 10 * time.Millisecond, Debounce: 20 * time.Millisecond}, Snapshot{"/source.go": digest(1)})
				defer consumer.stop(t)
				release := make(chan struct{})
				failure := errors.New("invalid configuration after rendering")
				consumer.generate = func(ctx context.Context) error {
					if len(consumer.generations) == 1 {
						select {
						case <-release:
						case <-ctx.Done():
							return ctx.Err()
						}
						return &ObservationError{Err: failure}
					}
					return nil
				}
				consumer.snapshot["/source.go"] = digest(2)
				advance(30 * time.Millisecond)
				if len(consumer.generations) != 1 {
					t.Fatalf("changed input did not begin generation: %#v", consumer.generations)
				}

				// The edited source was rendered, but malformed configuration
				// prevented publication and remains unreadable afterward.
				consumer.observeError = failure
				close(release)
				synctest.Wait()
				advance(100 * time.Millisecond)
				if len(consumer.generations) != 1 || len(consumer.reports) == 0 {
					t.Fatalf("invalid observation retried or lost its failure: generations=%#v reports=%v", consumer.generations, consumer.reports)
				}
				for _, err := range consumer.reports {
					if !errors.Is(err, failure) {
						t.Fatalf("unexpected failure while awaiting recovery: %v", err)
					}
				}
				if cancelWhileInvalid {
					return
				}

				// Restoring the original configuration yields the exact snapshot
				// used by the unpublished attempt, not another input change.
				consumer.observeError = nil
				advance(10 * time.Millisecond)
				want := []Snapshot{
					{"/source.go": digest(2)},
					{"/source.go": digest(2)},
				}
				if !reflect.DeepEqual(consumer.generations, want) {
					t.Fatalf("valid recovery lost unpublished input: generations=%#v, want %#v", consumer.generations, want)
				}
				reports := len(consumer.reports)
				advance(100 * time.Millisecond)
				if len(consumer.generations) != 2 || len(consumer.reports) != reports {
					t.Fatalf("recovered input retried or reported again: generations=%#v reports=%v", consumer.generations, consumer.reports)
				}
			})
		})
	}
}

func TestRunObservationErrorsSuppressAndRecover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		consumer := startConsumer(Options{Interval: time.Second}, Snapshot{"/source.go": digest(1)})
		defer consumer.stop(t)
		consumer.observeError = errors.New("unreadable source")
		consumer.snapshot = Snapshot{"/partial.go": digest(2)}
		advance(4 * time.Second)
		if len(consumer.reports) != 1 || len(consumer.generations) != 0 {
			t.Fatalf("observation error repeated or replaced stable input: reports=%v generations=%#v", consumer.reports, consumer.generations)
		}
		consumer.observeError = errors.New("invalid configuration")
		advance(3 * time.Second)
		consumer.observeError = errors.New("unreadable source")
		advance(time.Second)
		if len(consumer.reports) != 3 {
			t.Fatalf("distinct error transitions reported %d times, want 3", len(consumer.reports))
		}
		consumer.observeError = nil
		consumer.snapshot = Snapshot{"/source.go": digest(1)}
		advance(time.Second)
		if len(consumer.generations) != 0 {
			t.Fatal("recovery to the previous valid input generated output")
		}
		consumer.observeError = errors.New("unreadable source")
		advance(time.Second)
		if len(consumer.reports) != 4 {
			t.Fatal("successful recovery did not reset error suppression")
		}
		consumer.observeError = nil
		consumer.snapshot["/source.go"] = digest(3)
		advance(time.Second)
		if len(consumer.generations) != 1 || consumer.generations[0]["/source.go"] != digest(3) {
			t.Fatalf("changed recovered input was not generated: %#v", consumer.generations)
		}
	})
}

func TestRunObservationFailurePausesPendingGeneration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		consumer := startConsumer(Options{Interval: 10 * time.Millisecond, Debounce: 30 * time.Millisecond}, Snapshot{"/source.go": digest(1)})
		defer consumer.stop(t)
		consumer.snapshot["/source.go"] = digest(2)
		advance(10 * time.Millisecond)
		consumer.observeError = errors.New("read interrupted")
		advance(100 * time.Millisecond)
		if len(consumer.generations) != 0 || len(consumer.reports) != 1 {
			t.Fatalf("invalid observation allowed pending generation: generations=%d reports=%v", len(consumer.generations), consumer.reports)
		}
		consumer.observeError = nil
		advance(10 * time.Millisecond)
		if len(consumer.generations) != 1 || consumer.generations[0]["/source.go"] != digest(2) {
			t.Fatalf("recovered pending change was lost: %#v", consumer.generations)
		}
	})
}

func TestRunInitialObservationRecoveryOnlyEstablishesBaseline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		observeError := errors.New("initial read failed")
		snapshot := Snapshot{"/source.go": digest(1)}
		var generations, reports int
		result := make(chan error, 1)
		go func() {
			result <- Run(ctx, Options{Interval: time.Second}, func() (Snapshot, error) {
				return snapshot, observeError
			}, func(context.Context) error {
				generations++
				return nil
			}, func(error) { reports++ })
		}()
		synctest.Wait()
		advance(3 * time.Second)
		observeError = nil
		advance(time.Second)
		if generations != 0 || reports != 1 {
			t.Fatalf("initial recovery generated or repeated error: generations=%d reports=%d", generations, reports)
		}
		snapshot["/source.go"] = digest(2)
		advance(time.Second)
		if generations != 1 {
			t.Fatal("post-recovery input change was not generated")
		}
		cancel()
		synctest.Wait()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("Run cancellation = %v", err)
		}
	})
}

func TestRunCancellationStopsPendingAndBusyGeneration(t *testing.T) {
	for _, busy := range []bool{false, true} {
		name := "pending"
		if busy {
			name = "busy"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				consumer := startConsumer(Options{Interval: time.Second, Debounce: time.Second}, Snapshot{"/source.go": digest(1)})
				consumer.generate = func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				}
				consumer.snapshot["/source.go"] = digest(2)
				advance(time.Second)
				if busy {
					advance(time.Second)
					consumer.snapshot["/source.go"] = digest(3)
				}
				consumer.stop(t)
				wantGenerations := 0
				if busy {
					wantGenerations = 1
				}
				if len(consumer.generations) != wantGenerations || len(consumer.reports) != 0 {
					t.Fatalf("cancellation generated or reported failure: generations=%d reports=%v", len(consumer.generations), consumer.reports)
				}
			})
		})
	}
}

func TestRunRejectsInvalidTimingBeforeCallbacks(t *testing.T) {
	for _, opts := range []Options{
		{},
		{Interval: -time.Second},
		{Interval: time.Second, Debounce: -time.Nanosecond},
	} {
		called := false
		err := Run(context.Background(), opts, func() (Snapshot, error) {
			called = true
			return nil, nil
		}, func(context.Context) error {
			called = true
			return nil
		}, func(error) { called = true })
		if err == nil || called {
			t.Errorf("options %#v: err=%v callbacks called=%v", opts, err, called)
		}
	}
}

func TestRunCanceledContextDoesNotObserve(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := Run(ctx, Options{Interval: time.Second}, func() (Snapshot, error) {
		called = true
		return nil, nil
	}, func(context.Context) error {
		called = true
		return nil
	}, func(error) { called = true })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("canceled Run: err=%v callbacks called=%v", err, called)
	}
}
