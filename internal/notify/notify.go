// Package notify delivers Application lifecycle notifications to webhook and
// Slack sinks without blocking reconciliation.
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

// SignatureHeader carries `sha256=<hex HMAC-SHA256 of the body>` on webhook deliveries.
const SignatureHeader = "X-Solder-Signature"

var deliveries = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "solder_notification_deliveries_total",
	Help: "Notification deliveries by sink type and result (delivered, failed, dropped).",
}, []string{"type", "result"})

func init() {
	metrics.Registry.MustRegister(deliveries)
}

// Message is the redacted content of one notification.
type Message struct {
	Event          corev1alpha1.NotificationEvent `json:"event"`
	Application    string                         `json:"application"`
	Namespace      string                         `json:"namespace"`
	Revision       string                         `json:"revision,omitempty"`
	SourceRevision string                         `json:"sourceRevision,omitempty"`
	Message        string                         `json:"message,omitempty"`
	Plan           corev1alpha1.PlanSummary       `json:"plan"`
	ApproveCommand string                         `json:"approveCommand,omitempty"`
	Time           time.Time                      `json:"time"`
}

// Target is a resolved sink.
type Target struct {
	Type    corev1alpha1.NotificationSinkType
	URL     string
	HMACKey string
}

// Delivery is one message for one target. OnFailure, when set, is called
// after every attempt has failed.
type Delivery struct {
	Target    Target
	Message   Message
	OnFailure func(error)
}

// Dispatcher queues deliveries in memory and sends them from background
// workers. Deliveries are best effort: a full queue drops new ones, and
// queued ones are lost on restart.
type Dispatcher struct {
	client   *http.Client
	queue    chan Delivery
	workers  int
	attempts int
	backoff  time.Duration
}

// NewDispatcher returns a Dispatcher with a queue of the given size.
func NewDispatcher(client *http.Client, queueSize int) *Dispatcher {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Dispatcher{client: client, queue: make(chan Delivery, queueSize), workers: 2, attempts: 3, backoff: time.Second}
}

// Enqueue schedules a delivery without blocking and reports whether it was queued.
func (d *Dispatcher) Enqueue(delivery Delivery) bool {
	select {
	case d.queue <- delivery:
		return true
	default:
		deliveries.WithLabelValues(string(delivery.Target.Type), "dropped").Inc()
		return false
	}
}

// Start runs the delivery workers until ctx is done; it is a manager.Runnable.
func (d *Dispatcher) Start(ctx context.Context) error {
	for range d.workers {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case delivery := <-d.queue:
					d.deliver(ctx, delivery)
				}
			}
		}()
	}
	<-ctx.Done()
	return nil
}

func (d *Dispatcher) deliver(ctx context.Context, delivery Delivery) {
	var err error
	for attempt := range d.attempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d.backoff << (attempt - 1)):
			}
		}
		if err = d.send(ctx, delivery); err == nil {
			deliveries.WithLabelValues(string(delivery.Target.Type), "delivered").Inc()
			return
		}
	}
	deliveries.WithLabelValues(string(delivery.Target.Type), "failed").Inc()
	if delivery.OnFailure != nil {
		delivery.OnFailure(err)
	}
}

func (d *Dispatcher) send(ctx context.Context, delivery Delivery) error {
	body, err := Body(delivery.Target.Type, delivery.Message)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.Target.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if delivery.Target.Type == corev1alpha1.NotificationSinkWebhook {
		req.Header.Set("X-Solder-Event", string(delivery.Message.Event))
		req.Header.Set(SignatureHeader, Sign(delivery.Target.HMACKey, body))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("sink responded %s", resp.Status)
	}
	return nil
}

// Sign returns the signature header value for body.
func Sign(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Body renders a message for a sink type.
func Body(sinkType corev1alpha1.NotificationSinkType, msg Message) ([]byte, error) {
	if sinkType != corev1alpha1.NotificationSinkSlack {
		return json.Marshal(msg)
	}
	text := fmt.Sprintf("*%s/%s*: %s", msg.Namespace, msg.Application, msg.Event)
	if msg.SourceRevision != "" {
		text += fmt.Sprintf(" at `%s`", msg.SourceRevision)
	}
	if msg.Message != "" {
		text += "\n" + msg.Message
	}
	if msg.ApproveCommand != "" {
		text += fmt.Sprintf("\nPlan: %d create, %d update, %d delete. Approve with `%s`", msg.Plan.Create, msg.Plan.Update, msg.Plan.Delete, msg.ApproveCommand)
	}
	return json.Marshal(map[string]string{"text": text})
}
