/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/notify"
)

var _ = Describe("Application notifications", func() {
	const appName = "notified-app"
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}
	var (
		server   *httptest.Server
		received chan notify.Message
		stop     context.CancelFunc
		notifier *notify.Dispatcher
	)

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		received = make(chan notify.Message, 10)
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if r.Header.Get(notify.SignatureHeader) != notify.Sign("sink-key", body) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var msg notify.Message
			_ = json.Unmarshal(body, &msg)
			received <- msg
		}))
		notifier = notify.NewDispatcher(server.Client(), 10)
		var dispatchCtx context.Context
		dispatchCtx, stop = context.WithCancel(ctx)
		go func() { _ = notifier.Start(dispatchCtx) }()

		createRepository(ctx)
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "sink-secret", Namespace: "default"},
			Data:       map[string][]byte{"url": []byte(server.URL), "hmacKey": []byte("sink-key")},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1alpha1.NotificationSink{
			ObjectMeta: metav1.ObjectMeta{Name: "audit", Namespace: "default"},
			Spec:       corev1alpha1.NotificationSinkSpec{Type: corev1alpha1.NotificationSinkWebhook, SecretRef: corev1alpha1.SecretReference{Name: "sink-secret"}},
		})).To(Succeed())
	})

	AfterEach(func() {
		stop()
		server.Close()
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.NotificationSink{ObjectMeta: metav1.ObjectMeta{Name: "audit", Namespace: "default"}})
		deleteObject(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sink-secret", Namespace: "default"}})
		deleteApplicationRevisions(ctx, appName)
	})

	subscribedApplication := func(sink string, events ...corev1alpha1.NotificationEvent) {
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Notifications = []corev1alpha1.NotificationSubscription{{SinkRef: corev1alpha1.LocalObjectReference{Name: sink}, Events: events}}
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
	}

	It("sends one signed AwaitingApproval notification with the approve command", func() {
		subscribedApplication("audit", corev1alpha1.NotificationAwaitingApproval)
		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		reconciler.Notifier = notifier
		for range 2 {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
		}

		var msg notify.Message
		Eventually(received, 5*time.Second).Should(Receive(&msg))
		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(msg.Event).To(Equal(corev1alpha1.NotificationAwaitingApproval))
		Expect(msg.ApproveCommand).To(Equal("ksync approve " + appName + " -n default --revision " + revision.Name))
		Expect(msg.Plan.Create).To(Equal(int32(1)))
		Consistently(received, time.Second).ShouldNot(Receive(), "a steady AwaitingApproval state was notified twice")

		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		Expect(apimeta.IsStatusConditionTrue(app.Status.Conditions, "NotificationsReady")).To(BeTrue())
	})

	It("redacts secrets from failure notifications", func() {
		subscribedApplication("audit", corev1alpha1.NotificationFailed)
		reconciler := newApplicationReconciler(nil, &capturingRenderer{err: errors.New("render failed: token=super-secret-token")})
		reconciler.Notifier = notifier
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		var msg notify.Message
		Eventually(received, 5*time.Second).Should(Receive(&msg))
		Expect(msg.Event).To(Equal(corev1alpha1.NotificationFailed))
		Expect(msg.Message).NotTo(ContainSubstring("super-secret-token"))
	})

	It("reports a missing sink as a condition without blocking reconciliation", func() {
		subscribedApplication("missing", corev1alpha1.NotificationAwaitingApproval)
		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		reconciler.Notifier = notifier
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		Expect(listApplicationRevisions(ctx, appName).Items[0].Status.Phase).To(Equal(corev1alpha1.RevisionPhaseAwaitingApproval))
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		condition := apimeta.FindStatusCondition(app.Status.Conditions, "NotificationsReady")
		Expect(condition).NotTo(BeNil())
		Expect(condition.Reason).To(Equal("SinkInvalid"))
		Expect(condition.Message).To(ContainSubstring("missing"))
	})
})
