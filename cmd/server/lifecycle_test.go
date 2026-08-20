package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
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
