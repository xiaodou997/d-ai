package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestShutdownStackClosesInReverseOrderAndOnlyOnce(t *testing.T) {
	var got []string
	stack := shutdownStack{}
	stack.Add("postgres", func(context.Context) error {
		got = append(got, "postgres")
		return nil
	})
	stack.Add("redis", func(context.Context) error {
		got = append(got, "redis")
		return nil
	})

	if err := stack.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stack.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if want := []string{"redis", "postgres"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("close order = %v, want %v", got, want)
	}
}

func TestShutdownStackJoinsErrorsAndRejectsLateResources(t *testing.T) {
	firstErr := errors.New("first")
	secondErr := errors.New("second")
	var got []string
	stack := shutdownStack{}
	stack.Add("first", func(context.Context) error {
		got = append(got, "first")
		return firstErr
	})
	stack.Add("second", func(context.Context) error {
		got = append(got, "second")
		return secondErr
	})
	if err := stack.Close(context.Background()); !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("Close() error = %v, want both close errors", err)
	}
	stack.Add("late", func(context.Context) error {
		got = append(got, "late")
		return nil
	})
	if want := []string{"second", "first"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("close calls = %v, want %v", got, want)
	}
}

func TestPeriodicWorkerStopsIdempotently(t *testing.T) {
	started := make(chan struct{})
	worker := startHourlyCleanup(context.Background(), func(context.Context) {
		select {
		case <-started:
		default:
			close(started)
		}
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("periodic worker did not run its initial cleanup")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestPeriodicWorkerStopWaitsForInFlightCleanup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	worker := startHourlyCleanup(context.Background(), func(context.Context) {
		close(started)
		<-release
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("periodic worker did not enter cleanup")
	}

	shortCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := worker.Stop(shortCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("short Stop() error = %v, want deadline exceeded", err)
	}
	close(release)
	longCtx, longCancel := context.WithTimeout(context.Background(), time.Second)
	defer longCancel()
	if err := worker.Stop(longCtx); err != nil {
		t.Fatalf("long Stop() error = %v", err)
	}
}
