package wsserver

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"ws-rat/internal/model"
)

func powerClient(id string) *Client { return &Client{Info: model.AgentInfo{ID: id}} }

func pendingPowerCount(tracker *PowerTracker) int {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return len(tracker.pending)
}

func awaitPowerResult(t *testing.T, results <-chan any) any {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(time.Second):
		t.Fatal("missing power completion")
		return nil
	}
}

func TestPowerTrackerConnectionIdentityAndDuplicateResults(t *testing.T) {
	tracker := NewPowerTracker()
	f := &FrontendClient{}
	defer tracker.RemoveFrontend(f)
	a, b := powerClient("a"), powerClient("b")
	results := make(chan any, 3)
	_, err := tracker.Register(PowerRequest{Action: "shutdown_room", RequestID: "request", RoomID: "room"}, f, []*Client{a, b}, 3, func(v any) { results <- v })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.Register(PowerRequest{RequestID: "request"}, &FrontendClient{}, []*Client{a}, 1, func(any) {}); err == nil {
		t.Fatal("duplicate request accepted")
	}
	for _, client := range []*Client{powerClient("unknown"), powerClient("a")} {
		if tracker.Resolve(client, PowerResult{RequestID: "request", Success: true}) {
			t.Fatal("wrong connection resolved target")
		}
	}
	if tracker.Resolve(a, PowerResult{RequestID: "unknown", Success: true}) {
		t.Fatal("unknown request resolved")
	}
	if !tracker.Resolve(a, PowerResult{RequestID: "request", Success: true}) {
		t.Fatal("valid result ignored")
	}
	if tracker.Resolve(a, PowerResult{RequestID: "request", Success: false}) {
		t.Fatal("duplicate result counted")
	}
	if !tracker.Resolve(b, PowerResult{RequestID: "request", Success: false}) {
		t.Fatal("failure ignored")
	}
	r := awaitPowerResult(t, results).(PowerRoomResult)
	if r.Total != 3 || r.Online != 2 || r.Offline != 1 || r.Accepted != 1 || r.Failed != 1 || r.Timeout != 0 {
		t.Fatalf("bad aggregate: %+v", r)
	}
	if pendingPowerCount(tracker) != 0 {
		t.Fatal("completed request leaked")
	}
	if tracker.Resolve(a, PowerResult{RequestID: "request"}) {
		t.Fatal("expired request resolved")
	}
}

func TestPowerTrackerTimeout(t *testing.T) {
	for _, action := range []string{"shutdown", "shutdown_room"} {
		t.Run(action, func(t *testing.T) {
			tracker := NewPowerTracker()
			tracker.timeout = 20 * time.Millisecond
			f := &FrontendClient{}
			defer tracker.RemoveFrontend(f)
			a, b := powerClient("a"), powerClient("b")
			clients := []*Client{a}
			if action == "shutdown_room" {
				clients = append(clients, b)
			}
			results := make(chan any, 1)
			p, err := tracker.Register(PowerRequest{Action: action, RequestID: "r", AgentID: "a", RoomID: "room"}, f, clients, len(clients), func(v any) { results <- v })
			if err != nil {
				t.Fatal(err)
			}
			if action == "shutdown_room" {
				tracker.Resolve(a, PowerResult{RequestID: "r", Success: true})
			}
			v := awaitPowerResult(t, results)
			switch r := v.(type) {
			case PowerResult:
				if r.Success || r.Code != "timeout" || r.AgentID != "a" {
					t.Fatalf("bad timeout: %+v", r)
				}
			case PowerRoomResult:
				if r.Accepted != 1 || r.Timeout != 1 || r.Failed != 0 {
					t.Fatalf("bad aggregate: %+v", r)
				}
			}
			if pendingPowerCount(tracker) != 0 || p.ctx.Err() == nil {
				t.Fatal("timeout did not clean up")
			}
			if tracker.Resolve(a, PowerResult{RequestID: "r"}) {
				t.Fatal("late result accepted")
			}
		})
	}
}

func TestPowerTrackerFrontendCleanup(t *testing.T) {
	tracker := NewPowerTracker()
	f, g := &FrontendClient{}, &FrontendClient{}
	defer tracker.RemoveFrontend(g)
	results := make(chan any, 2)
	a := powerClient("a")
	p, _ := tracker.Register(PowerRequest{Action: "shutdown", RequestID: "first"}, f, []*Client{a}, 1, func(v any) { results <- v })
	tracker.Register(PowerRequest{Action: "shutdown", RequestID: "second"}, g, []*Client{a}, 1, func(v any) { results <- v })
	tracker.RemoveFrontend(f)
	tracker.expire(p)
	if p.ctx.Err() == nil || pendingPowerCount(tracker) != 1 {
		t.Fatal("incorrect cleanup")
	}
	if tracker.Resolve(a, PowerResult{RequestID: "first"}) {
		t.Fatal("removed request resolved")
	}
	if !tracker.Resolve(a, PowerResult{RequestID: "second"}) {
		t.Fatal("other frontend affected")
	}
	r := awaitPowerResult(t, results).(PowerResult)
	if r.RequestID != "second" || len(results) != 0 {
		t.Fatal("removed frontend received result")
	}
}

func TestPowerTrackerConcurrentRequestsAndCompletion(t *testing.T) {
	tracker := NewPowerTracker()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("request-%d", i)
			f, a := &FrontendClient{}, powerClient("agent")
			results := make(chan any, 2)
			p, err := tracker.Register(PowerRequest{Action: "shutdown", RequestID: id}, f, []*Client{a}, 1, func(v any) { results <- v })
			if err != nil {
				t.Error(err)
				return
			}
			var race sync.WaitGroup
			race.Add(2)
			go func() { defer race.Done(); tracker.Resolve(a, PowerResult{RequestID: id, Success: true}) }()
			go func() { defer race.Done(); tracker.expire(p) }()
			race.Wait()
			if len(results) != 1 {
				t.Errorf("request %s completed %d times", id, len(results))
				return
			}
			if r := (<-results).(PowerResult); r.RequestID != id {
				t.Errorf("wrong owner: %+v", r)
			}
		}(i)
	}
	wg.Wait()
	if pendingPowerCount(tracker) != 0 {
		t.Fatal("concurrent requests leaked")
	}
}
