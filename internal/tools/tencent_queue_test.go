package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestQQReadConcurrencySpacingAndQueuedCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started := make(chan time.Time, 4)
		finished := make(chan error, 3)
		client := &Client{run: func(ctx context.Context, _ ...string) (json.RawMessage, error) {
			started <- time.Now()
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		var previous time.Time
		for i := 0; i < 3; i++ {
			go func() { _, err := client.read(ctx, func(string) {}); finished <- err }()
			at := <-started
			if !previous.IsZero() && at.Sub(previous) < 600*time.Millisecond {
				t.Fatal("QQ starts bypassed the shared 600ms interval")
			}
			previous = at
		}
		waiterCtx, cancelWaiter := context.WithCancel(ctx)
		defer cancelWaiter()
		waiterDone := make(chan error, 1)
		go func() { _, err := client.read(waiterCtx, func(string) {}); waiterDone <- err }()
		synctest.Wait()
		select {
		case <-started:
			t.Fatal("more than three reads ran concurrently")
		default:
		}
		cancelWaiter()
		if err := <-waiterDone; !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting cancellation: %v", err)
		}
		select {
		case <-finished:
			t.Fatal("cancelling a waiter stopped another request")
		default:
		}
		cancel()
		for i := 0; i < 3; i++ {
			if err := <-finished; !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		}
	})
}

func TestQQRateLimitDefersExistingWaitersAndSurvivesCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ownerCtx, cancelOwner := context.WithCancel(context.Background())
		defer cancelOwner()
		firstStarted := make(chan struct{})
		rateLimited := make(chan struct{})
		limitedProgress := make(chan struct{})
		secondStarted := make(chan time.Time, 1)
		client := &Client{run: func(_ context.Context, args ...string) (json.RawMessage, error) {
			if args[0] == "owner" {
				close(firstStarted)
				<-rateLimited
				return nil, errors.New("rate_limited")
			}
			secondStarted <- time.Now()
			return json.RawMessage(`{}`), nil
		}}
		ownerDone := make(chan error, 1)
		go func() {
			_, err := client.read(ownerCtx, func(string) { close(limitedProgress) }, "owner")
			ownerDone <- err
		}()
		<-firstStarted
		waiterDone := make(chan error, 1)
		go func() {
			_, err := client.read(context.Background(), func(string) {}, "waiting-before-limit")
			waiterDone <- err
		}()
		synctest.Wait() // The second request is already waiting on the original 600ms delay.
		limitedAt := time.Now()
		close(rateLimited)
		<-limitedProgress
		time.Sleep(time.Second)
		select {
		case <-secondStarted:
			t.Fatal("old spacing reservation bypassed the new cooldown")
		default:
		}
		cancelOwner()
		if err := <-ownerDone; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		newCtx, cancelNew := context.WithCancel(context.Background())
		defer cancelNew()
		newDone := make(chan error, 1)
		go func() { _, err := client.read(newCtx, func(string) {}, "waiting-after-limit"); newDone <- err }()
		synctest.Wait()
		cancelNew()
		if err := <-newDone; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		time.Sleep(68 * time.Second)
		select {
		case <-secondStarted:
			t.Fatal("cancellation cleared the shared 70s cooldown")
		default:
		}
		if at := <-secondStarted; at.Sub(limitedAt) < 70*time.Second {
			t.Fatal("read started before global cooldown elapsed")
		}
		if err := <-waiterDone; err != nil {
			t.Fatal(err)
		}
	})
}

func TestQQReadRetriesAtMostOnceAndKeepsSecondLimitCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attempts []time.Time
		client := &Client{run: func(context.Context, ...string) (json.RawMessage, error) {
			attempts = append(attempts, time.Now())
			return nil, errors.New("rate_limited")
		}}
		if _, err := client.read(context.Background(), func(string) {}); err == nil || err.Error() != "频道仍在限流，请稍后再试" {
			t.Fatalf("wrong final rate-limit error: %v", err)
		}
		if len(attempts) != 2 || attempts[1].Sub(attempts[0]) < 70*time.Second {
			t.Fatal("retry count or delay changed")
		}
		client.run = func(context.Context, ...string) (json.RawMessage, error) {
			if time.Since(attempts[1]) < 70*time.Second {
				t.Fatal("second rate limit did not cool down the account")
			}
			return json.RawMessage(`{}`), nil
		}
		if _, err := client.read(context.Background(), func(string) {}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestQQCancelledReadDoesNotConsumeAvailableQueueSlot(t *testing.T) {
	var calls int
	client := &Client{run: func(context.Context, ...string) (json.RawMessage, error) { calls++; return json.RawMessage(`{}`), nil }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.read(ctx, func(string) {}, "cancelled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled entry accepted: %v", err)
	}
	if calls != 0 {
		t.Fatal("cancelled entry ran a command")
	}
	if _, err := client.read(context.Background(), func(string) {}, "next"); err != nil || calls != 1 {
		t.Fatal("queue slot was not released", err)
	}
}
