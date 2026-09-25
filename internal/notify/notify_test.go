package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

func approvalMessage() Message {
	return Message{
		Event: corev1alpha1.NotificationAwaitingApproval, Application: "payments", Namespace: "default",
		Revision: "payments-abc", SourceRevision: "abc123", Plan: corev1alpha1.PlanSummary{Create: 2},
		ApproveCommand: "ksync approve payments -n default --revision payments-abc",
	}
}

func fastDispatcher(client *http.Client, size int) *Dispatcher {
	d := NewDispatcher(client, size)
	d.backoff = time.Millisecond
	return d
}

func TestWebhookDeliveryIsSignedAndRetried(t *testing.T) {
	var calls atomic.Int32
	received := make(chan Message, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get(SignatureHeader) != Sign("shared-key", body) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var msg Message
		_ = json.Unmarshal(body, &msg)
		received <- msg
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := fastDispatcher(server.Client(), 4)
	go func() { _ = d.Start(ctx) }()
	d.Enqueue(Delivery{Target: Target{Type: corev1alpha1.NotificationSinkWebhook, URL: server.URL, HMACKey: "shared-key"}, Message: approvalMessage()})

	select {
	case msg := <-received:
		if msg.Event != corev1alpha1.NotificationAwaitingApproval || msg.ApproveCommand == "" {
			t.Fatalf("message = %#v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("signed delivery not received after retry")
	}
}

func TestFailedDeliveryReportsAfterAllAttempts(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := fastDispatcher(server.Client(), 4)
	go func() { _ = d.Start(ctx) }()
	failed := make(chan error, 1)
	d.Enqueue(Delivery{Target: Target{Type: corev1alpha1.NotificationSinkSlack, URL: server.URL}, Message: approvalMessage(), OnFailure: func(err error) { failed <- err }})

	select {
	case err := <-failed:
		if err == nil || calls.Load() != 3 {
			t.Fatalf("err = %v after %d calls", err, calls.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("failure was not reported")
	}
}

func TestEnqueueNeverBlocks(t *testing.T) {
	d := NewDispatcher(nil, 1)
	if !d.Enqueue(Delivery{Message: approvalMessage()}) {
		t.Fatal("first delivery was not queued")
	}
	done := make(chan bool, 1)
	go func() { done <- d.Enqueue(Delivery{Message: approvalMessage()}) }()
	select {
	case queued := <-done:
		if queued {
			t.Fatal("full queue accepted a delivery")
		}
	case <-time.After(time.Second):
		t.Fatal("Enqueue blocked on a full queue")
	}
}

func TestSlackBodyCarriesApproveCommand(t *testing.T) {
	body, err := Body(corev1alpha1.NotificationSinkSlack, approvalMessage())
	if err != nil {
		t.Fatal(err)
	}
	var slack map[string]string
	if err := json.Unmarshal(body, &slack); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(slack["text"], "ksync approve payments") || !strings.Contains(slack["text"], "2 create") {
		t.Fatalf("slack text = %q", slack["text"])
	}
}

func TestDeliveryErrorsNeverContainTheSinkURL(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	sink := server.URL + "/services/T000/B000/slack-secret-token"
	server.Close()
	d := NewDispatcher(nil, 1)
	for name, target := range map[string]string{"unreachable": sink, "unparseable": "https://hooks.example.com/slack-secret-token\x7f"} {
		err := d.send(context.Background(), Delivery{Target: Target{Type: corev1alpha1.NotificationSinkSlack, URL: target}, Message: approvalMessage()})
		if err == nil || strings.Contains(err.Error(), "slack-secret-token") {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}

func TestDefaultClientDoesNotFollowRedirects(t *testing.T) {
	var followed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed.Store(true) }))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	err := NewDispatcher(nil, 1).send(context.Background(), Delivery{Target: Target{Type: corev1alpha1.NotificationSinkWebhook, URL: redirector.URL}, Message: approvalMessage()})
	if err == nil || followed.Load() {
		t.Fatalf("redirect was followed: err = %v, followed = %v", err, followed.Load())
	}
}
