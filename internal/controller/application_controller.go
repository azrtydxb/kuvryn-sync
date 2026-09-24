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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	sigsyaml "sigs.k8s.io/yaml"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/applier"
	"github.com/azrtydxb/solder/internal/decrypt"
	"github.com/azrtydxb/solder/internal/health"
	"github.com/azrtydxb/solder/internal/history"
	"github.com/azrtydxb/solder/internal/impersonate"
	"github.com/azrtydxb/solder/internal/live"
	"github.com/azrtydxb/solder/internal/notify"
	"github.com/azrtydxb/solder/internal/ops"
	"github.com/azrtydxb/solder/internal/ordering"
	"github.com/azrtydxb/solder/internal/planner"
	"github.com/azrtydxb/solder/internal/prune"
	"github.com/azrtydxb/solder/internal/redact"
	"github.com/azrtydxb/solder/internal/renderer"
	helmrenderer "github.com/azrtydxb/solder/internal/renderer/helm"
	kustomizerenderer "github.com/azrtydxb/solder/internal/renderer/kustomize"
	yamlrenderer "github.com/azrtydxb/solder/internal/renderer/yaml"
	"github.com/azrtydxb/solder/internal/resource"
	"github.com/azrtydxb/solder/internal/retry"
	"github.com/azrtydxb/solder/internal/rollback"
	"github.com/azrtydxb/solder/internal/source"
	gitcache "github.com/azrtydxb/solder/internal/source/git"
	"github.com/azrtydxb/solder/internal/status"
	"github.com/azrtydxb/solder/internal/syncpolicy"
	"github.com/azrtydxb/solder/internal/validate"
)

const (
	defaultPlanResourceLimit = 50
	applicationFinalizer     = "applications.solder.io/finalizer"
)

// RendererFactory builds a renderer for an Application render type.
type RendererFactory func(corev1alpha1.RenderType) (renderer.Renderer, error)

// ApplicationReconciler reconciles an Application object.
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	SourceResolver source.Resolver
	CacheDir       string
	Renderers      RendererFactory
	PlanLimit      int
	Recorder       record.EventRecorder
	Tracer         ops.Tracer

	// Impersonation builds clients that act as an Application's service account.
	Impersonation *impersonate.Clients
	// DefaultServiceAccount is impersonated when an Application names none.
	DefaultServiceAccount string
	// DriftResyncInterval re-checks Applications that manage kinds the
	// controller may not watch. Zero disables the resync.
	DriftResyncInterval time.Duration
	// Notifier delivers lifecycle notifications; nil disables them.
	Notifier *notify.Dispatcher
	// ChartCAFile trusts an additional CA for chart repositories; only tests
	// set it today.
	ChartCAFile string

	watches      *driftWatches
	healthChecks healthCheckCache
}

// +kubebuilder:rbac:groups=solder.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solder.io,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solder.io,resources=applications/finalizers,verbs=update
// +kubebuilder:rbac:groups=solder.io,resources=repositories,verbs=get;list;watch
// +kubebuilder:rbac:groups=solder.io,resources=revisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solder.io,resources=revisions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=impersonate
// +kubebuilder:rbac:groups=solder.io,resources=healthchecks,verbs=get;list;watch
// +kubebuilder:rbac:groups=solder.io,resources=notificationsinks,verbs=get;list;watch
// Managed resources are read and changed as the Application's service account;
// the controller itself only watches their metadata to notice drift.
// +kubebuilder:rbac:groups="",resources=configmaps;services;secrets,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;daemonsets,verbs=list;watch

// Reconcile resolves, renders, validates, plans, applies approved changes, and observes health.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reconcileErr error) {
	start := time.Now()
	if r.Tracer != nil {
		var finish func(error)
		ctx, finish = r.Tracer.Start(ctx, "Application/Reconcile")
		defer func() { finish(reconcileErr) }()
	}
	metricApp := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: req.Namespace}}
	metricPhase := corev1alpha1.RevisionPhase("")
	defer func() { ops.ObserveReconcile(metricApp, metricPhase, reconcileErr, time.Since(start)) }()
	log := logf.FromContext(ctx)
	unwatched := false
	defer func() {
		if reconcileErr == nil && unwatched && result.RequeueAfter == 0 && r.DriftResyncInterval > 0 {
			result.RequeueAfter = r.DriftResyncInterval
		}
	}()

	application := &corev1alpha1.Application{}
	if err := r.Get(ctx, req.NamespacedName, application); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	metricApp = *application
	if !application.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, application)
	}
	if application.Spec.DeletionPolicy == corev1alpha1.DeletionPolicyDeleteManagedResources && !controllerutil.ContainsFinalizer(application, applicationFinalizer) {
		controllerutil.AddFinalizer(application, applicationFinalizer)
		return ctrl.Result{}, r.Update(ctx, application)
	}

	if application.Spec.Suspend {
		application.Status.ObservedGeneration = application.Generation
		application.Status.State = corev1alpha1.HealthStateSuspended
		application.Status.Health.State = corev1alpha1.HealthStateSuspended
		if application.Status.Sync.State == "" {
			application.Status.Sync.State = corev1alpha1.SyncStateUnknown
		}
		return ctrl.Result{}, r.Status().Update(ctx, application)
	}

	// The CRD refuses an invalid Helm release name, but an Application stored
	// before that rule keeps it, and the Revision CRD would then refuse every
	// Revision. Report it once instead of retrying a Create that cannot work.
	if err := validateHelmReleaseName(application); err != nil {
		r.markApplicationFailure(application, "ValidationFailure", err.Error())
		return ctrl.Result{}, r.Status().Update(ctx, application)
	}

	tenant, err := r.tenantClient(application)
	if err != nil {
		reason := "ServiceAccountFailure"
		if errors.Is(err, errServiceAccountRequired) {
			reason = "ServiceAccountRequired"
		}
		r.markApplicationFailure(application, reason, safeMessage(err, "Application service account could not be used"))
		return ctrl.Result{}, r.Status().Update(ctx, application)
	}

	repository := &corev1alpha1.Repository{}
	repoKey := client.ObjectKey{Namespace: application.Namespace, Name: application.Spec.Source.RepositoryRef.Name}
	if err := r.Get(ctx, repoKey, repository); err != nil {
		if apierrors.IsNotFound(err) {
			r.markApplicationFailure(application, "SourceFailure", "Referenced Repository was not found")
			return ctrl.Result{}, r.Status().Update(ctx, application)
		}
		return ctrl.Result{}, err
	}
	if repository.Spec.Type != "" && repository.Spec.Type != corev1alpha1.RepositoryTypeGit {
		r.markApplicationFailure(application, "SourceFailure", "Only Git repositories are supported")
		return ctrl.Result{}, r.Status().Update(ctx, application)
	}
	if repository.Spec.Git == nil {
		r.markApplicationFailure(application, "SourceFailure", "Git repository configuration is required")
		return ctrl.Result{}, r.Status().Update(ctx, application)
	}

	credentials, err := (&RepositoryReconciler{Client: r.Client}).loadGitCredentials(ctx, repository)
	if err != nil {
		r.markApplicationFailure(application, failureReason(err, "SourceFailure"), safeMessage(err, "Application source authentication failed"))
		return ctrl.Result{}, r.Status().Update(ctx, application)
	}

	ref := strings.TrimSpace(application.Spec.Source.Revision)
	if rollbackRef := strings.TrimSpace(application.GetAnnotations()["solder.io/rollback-revision"]); rollbackRef != "" {
		ref = rollbackRef
	}
	if ref == "" {
		ref = repository.Spec.Git.Revision
	}
	resolved, err := r.resolver().Resolve(ctx, source.GitRepository{
		URL:      repository.Spec.Git.URL,
		Revision: ref,
		Auth:     credentials,
	})
	if err != nil {
		r.markApplicationFailure(application, failureReason(err, "SourceFailure"), safeMessage(err, "Application source resolution failed"))
		return ctrl.Result{}, r.Status().Update(ctx, application)
	}

	// Drift of a finished rollout is reported without re-evaluating health, so
	// it keeps the health last observed.
	previousHealth, previousState := application.Status.Health.State, application.Status.State
	application.Status.ObservedGeneration = application.Generation
	application.Status.DesiredRevision = resolved.Revision
	application.Status.State = corev1alpha1.HealthStateUnknown
	application.Status.Health.State = corev1alpha1.HealthStateUnknown
	application.Status.Sync.State = corev1alpha1.SyncStatePlanning

	revision, err := r.ensureRevision(ctx, application, resolved.Revision)
	if err != nil {
		return ctrl.Result{}, err
	}
	metricPhase = revision.Status.Phase
	// previousPhase is the persisted phase, so notifications fire only on
	// transitions rather than on every reconcile of a steady state.
	previousPhase := revision.Status.Phase
	if blocked := retryBlocked(application, revision); blocked != nil {
		return ctrl.Result{}, r.reportRetryBlocked(ctx, application, revision, *blocked)
	}
	// Whether a rollout is finished comes from the Revision, not from its
	// phase, which planning, approval, dependency waits and failures overwrite.
	rollout := rolloutOf(revision, previousPhase)
	// A rollout starts over, running its hooks again, unless it is still in
	// progress or this Revision is the one deployed and it finished.
	fresh := rollout == rolloutNotStarted || (rollout == rolloutComplete && application.Status.DeployedRevision != resolved.Revision)
	if fresh {
		revision.Status.Hooks = nil
	}

	now := metav1.Now()
	if revision.Status.Phase == "" || revision.Status.Phase == corev1alpha1.RevisionPhasePending || revision.Status.Phase == corev1alpha1.RevisionPhaseFailed {
		revision.Status.Attempts++
	}
	status.StartPlanning(revision, application, now)
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return ctrl.Result{}, err
	}

	rendered, helmInputs, failure := r.renderDesired(ctx, tenant, application, resolved)
	if failure != nil {
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, *failure)
	}
	// A hook value Solder does not know is refused rather than guessed at.
	if err := ordering.ValidateHooks(rendered); err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "ValidationFailure", Message: safeMessage(err, "Rendered hook annotation is invalid"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	// Test, delete and rollback hooks are never deployed, so they are not
	// desired state.
	rendered = slices.DeleteFunc(rendered, func(obj unstructured.Unstructured) bool {
		return ordering.Hook(obj) == ordering.StageSkip
	})
	if err := defaultDestinationNamespace(rendered, application.Spec.Destination.Namespace, resource.NewScopes(r.RESTMapper(), rendered)); err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "ValidationFailure", Message: safeMessage(err, "Rendered resource scope could not be resolved"), Retryable: true}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	if desiredHash, err := desiredStateHash(rendered); err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "PlanFailure", Message: safeMessage(err, "Desired state could not be fingerprinted"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	} else if desiredHash != revision.Spec.DesiredStateHash {
		revision.Spec.DesiredStateHash = desiredHash
		if err := r.Update(ctx, revision); err != nil {
			return ctrl.Result{}, err
		}
	}
	unwatched = !r.ensureWatches(ctx, applier.UnionKinds(objectKinds(rendered), applier.InventoryKinds(application)))
	if err := validate.Desired(rendered, validate.Options{DestinationNamespace: application.Spec.Destination.Namespace}); err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "ValidationFailure", Message: safeMessage(err, "Rendered desired state is invalid"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}

	// Plan against objects carrying the metadata the applier will add, so it
	// is not reported as drift. Copies keep the renderer's output untouched.
	marked := make([]unstructured.Unstructured, len(rendered))
	for i := range rendered {
		marked[i] = *rendered[i].DeepCopy()
		applier.MarkManaged(&marked[i], application.Name, application.Namespace, revision.Name)
	}
	rendered = marked
	// Hooks that already succeeded for this Revision are not planned: once
	// cleaned up they would otherwise look like drift and run again. They
	// stay desired, so they are never pruned.
	hooks := hookStates(revision.Status.Hooks)
	planned := withoutCompletedHooks(rendered, hooks)
	liveResult, err := (live.Reader{Client: tenant}).Read(ctx, planned)
	if err != nil {
		failure := accessFailure(err, "PlanFailure", "Live state could not be read", true)
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	liveObjects := slices.Collect(maps.Values(liveResult.Found))
	managedStale := []unstructured.Unstructured{}
	if application.Spec.Sync.Prune {
		managed, skipped, err := applier.ListManaged(ctx, tenant, application, applier.ListOptions{DesiredKinds: objectKinds(rendered)})
		if err != nil {
			failure := accessFailure(err, "PlanFailure", "Managed resources could not be inventoried", true)
			return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
		}
		r.warnSkippedKinds(application, "PruneInventoryIncomplete", skipped)
		managedStale = staleManagedObjects(rendered, liveObjects, managed)
		liveObjects = append(liveObjects, managedStale...)
	}

	plan, err := planner.Build(planned, liveObjects)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "PlanFailure", Message: safeMessage(err, "Plan could not be built"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	markConflictPolicy(&plan, syncpolicy.EffectiveConflictPolicy(application.Spec.Sync))
	revision.Status.Plan = plan.RevisionPlan(r.planLimit())
	redactPlanValues(&revision.Status.Plan, helmInputs.secretValues)
	revision.Status.ChartDigest = helmInputs.chartDigest
	digest, err := planDigest(revision.Spec.DesiredStateHash, plan, helmInputs.secretValues)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "PlanFailure", Message: safeMessage(err, "Plan could not be fingerprinted"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	revision.Status.Plan.Digest = digest
	revision.Status.Failure = nil
	r.event(application, corev1.EventTypeNormal, "PlanCreated", "Application plan created")
	progress := rolloutProgress{previousPhase: previousPhase, state: rollout, hooks: hooks, previousHealth: previousHealth}
	if plan.Summary.Create == 0 && plan.Summary.Update == 0 && plan.Summary.Delete == 0 {
		// A rollout in progress is not done just because nothing is left to
		// apply: keep waiting until every group is Healthy.
		if rollout == rolloutInProgress {
			return r.applyAndObserve(ctx, tenant, application, revision, rendered, managedStale, progress)
		}
		application.Status.ManagedKinds = managedKinds(objectKinds(rendered))
		transition := previousPhase != corev1alpha1.RevisionPhaseHealthy && previousPhase != corev1alpha1.RevisionPhaseRolledBack
		if err := r.completeSuccessfulDeployment(ctx, application, revision, "Application already synced", transition); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Application already synced", "application", application.Name, "namespace", application.Namespace, "revision", resolved.Revision, "revisionRecord", revision.Name)
		return ctrl.Result{}, nil
	}
	// Differences after this Revision's rollout finished are drift, which
	// only self-heal repairs. Nothing is applied, so conflicts do not matter,
	// and the Revision keeps its finished phase so the next reconcile does
	// not mistake it for a rollout in progress.
	if !fresh && rollout == rolloutComplete && !application.Spec.Sync.SelfHeal {
		revision.Status.Phase = finishedPhase(previousPhase)
		application.Status.Sync.State = corev1alpha1.SyncStateDrifted
		application.Status.Health.State, application.Status.State = previousHealth, previousState
		if err := r.updateRevisionStatus(ctx, revision); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Status().Update(ctx, application); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Application drifted from its deployed Revision", "application", application.Name, "namespace", application.Namespace, "revision", resolved.Revision, "revisionRecord", revision.Name)
		return ctrl.Result{}, nil
	}
	if failure := conflictFailure(plan); failure != nil {
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, *failure)
	}
	if ready, err := r.dependenciesReady(ctx, application); err != nil {
		return ctrl.Result{}, err
	} else if !ready {
		if err := r.updateRevisionStatus(ctx, revision); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, r.Status().Update(ctx, application)
	}
	approval, stale := manualApproval(application, revision, rollout)
	if !application.Spec.Sync.Automatic && approval == nil {
		status.AwaitApproval(revision, application)
		if previousPhase != corev1alpha1.RevisionPhaseAwaitingApproval {
			r.notify(ctx, application, revision, corev1alpha1.NotificationAwaitingApproval, "Application plan is awaiting approval")
		}
		if err := r.updateRevisionStatus(ctx, revision); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Status().Update(ctx, application); err != nil {
			return ctrl.Result{}, err
		}
		if stale {
			r.event(application, corev1.EventTypeWarning, "ApprovalStale", "Application plan changed after approval; approve the new plan")
		} else {
			r.event(application, corev1.EventTypeNormal, "ApprovalRequired", "Application plan is awaiting approval")
		}
		log.Info("Planned Application", "application", application.Name, "namespace", application.Namespace, "revision", resolved.Revision, "revisionRecord", revision.Name)
		return ctrl.Result{}, nil
	}
	if !application.Spec.Sync.Automatic {
		revision.Status.Approval = approval
	}
	return r.applyAndObserve(ctx, tenant, application, revision, rendered, managedStale, progress)
}

// manualApproval returns the approval recorded for exactly this Revision and
// plan digest. stale reports an approval for this Revision whose plan has
// changed since, which must be approved again.
//
// Applying one group of a rollout changes the plan for the next, so an
// approval this rollout already acted on covers the rest of it, but only
// while the webhook-stamped approval still stands and the desired state is
// the one it was given for: new desired objects need a fresh approval.
func manualApproval(application *corev1alpha1.Application, revision *corev1alpha1.Revision, rollout rolloutState) (approval *corev1alpha1.RevisionApproval, stale bool) {
	annotations := application.GetAnnotations()
	if annotations[corev1alpha1.ApprovedRevisionAnnotation] != revision.Name {
		return nil, false
	}
	approvedBy := annotations[corev1alpha1.ApprovedByAnnotation]
	digest := annotations[corev1alpha1.ApprovedDigestAnnotation]
	approvedAt, err := time.Parse(time.RFC3339, annotations[corev1alpha1.ApprovedAtAnnotation])
	if approvedBy == "" || err != nil {
		return nil, true
	}
	if digest == revision.Status.Plan.Digest {
		return &corev1alpha1.RevisionApproval{ApprovedBy: approvedBy, ApprovedAt: metav1.NewTime(approvedAt), PlanDigest: digest, DesiredStateHash: revision.Spec.DesiredStateHash}, false
	}
	if recorded := revision.Status.Approval; recorded != nil && rollout == rolloutInProgress &&
		recorded.PlanDigest == digest && recorded.ApprovedBy == approvedBy &&
		recorded.DesiredStateHash != "" && recorded.DesiredStateHash == revision.Spec.DesiredStateHash {
		return recorded, false
	}
	return nil, true
}

// planDigest fingerprints what an approval covers: the rendered desired state
// and the complete, redacted change plan against live state. Secret-sourced
// Helm values are masked before hashing, so they never feed the digest in
// clear; a change to them still changes the digest through desiredStateHash,
// which covers the rendered objects they end up in.
func planDigest(desiredStateHash string, plan planner.Plan, secretValues []string) (string, error) {
	full := plan.RevisionPlan(0)
	redactPlanValues(&full, secretValues)
	raw, err := json.Marshal(full)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(desiredStateHash+"\n"), raw...))
	return hex.EncodeToString(sum[:]), nil
}

var errServiceAccountRequired = errors.New("application has no serviceAccountName and the controller has no default service account")

// tenantClient returns a client acting as the Application's effective service
// account, refusing when no service account is configured.
func (r *ApplicationReconciler) tenantClient(application *corev1alpha1.Application) (client.Client, error) {
	application.Status.ServiceAccountName = ""
	name := effectiveServiceAccount(application, r.DefaultServiceAccount)
	if name == "" {
		return nil, errServiceAccountRequired
	}
	if r.Impersonation == nil {
		return nil, fmt.Errorf("service account impersonation is not configured")
	}
	tenant, err := r.Impersonation.For(application.Namespace, name)
	if err != nil {
		return nil, err
	}
	application.Status.ServiceAccountName = name
	return tenant, nil
}

func effectiveServiceAccount(application *corev1alpha1.Application, defaultServiceAccount string) string {
	if application.Spec.ServiceAccountName != "" {
		return application.Spec.ServiceAccountName
	}
	return defaultServiceAccount
}

// healthEvaluator compiles the cluster's HealthChecks. Invalid rules, which
// admission normally rejects, are skipped and reported as a Warning Event.
func (r *ApplicationReconciler) healthEvaluator(ctx context.Context, application *corev1alpha1.Application) (health.Evaluator, error) {
	var checks corev1alpha1.HealthCheckList
	if err := r.List(ctx, &checks); err != nil {
		return health.Evaluator{}, err
	}
	evaluator, err := r.healthChecks.get(checks.Items)
	if err != nil {
		r.event(application, corev1.EventTypeWarning, "InvalidHealthCheck", safeMessage(err, "A HealthCheck rule is invalid"))
	}
	return evaluator, nil
}

// healthCheckCache keeps the Evaluator compiled from the cluster's
// HealthChecks and compiles again only when one is added, changed or removed.
// An Evaluator is read-only once built, so workers share it safely.
type healthCheckCache struct {
	mu        sync.Mutex
	compiled  bool
	key       string
	evaluator health.Evaluator
	err       error
	// compiles counts compilations, for tests.
	compiles int
}

func (c *healthCheckCache) get(checks []corev1alpha1.HealthCheck) (health.Evaluator, error) {
	versions := make([]string, 0, len(checks))
	for _, check := range checks {
		versions = append(versions, check.Name+"/"+string(check.UID)+"/"+check.ResourceVersion)
	}
	slices.Sort(versions)
	key := strings.Join(versions, ",")
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.compiled || c.key != key {
		c.evaluator, c.err = health.NewEvaluator(checks)
		c.compiled, c.key = true, key
		c.compiles++
	}
	return c.evaluator, c.err
}

// ensureWatches starts drift watches for kinds the controller may watch and
// reports whether every kind is watched. Without a running manager (unit
// tests) nothing is watched.
func (r *ApplicationReconciler) ensureWatches(ctx context.Context, kinds []schema.GroupVersionKind) bool {
	if r.watches == nil {
		return false
	}
	return r.watches.ensure(ctx, kinds)
}

// warnSkippedKinds records kinds the service account may not list, whose
// managed objects Solder therefore cannot find or delete.
func (r *ApplicationReconciler) warnSkippedKinds(application *corev1alpha1.Application, reason string, kinds []string) {
	if len(kinds) == 0 {
		return
	}
	r.event(application, corev1.EventTypeWarning, reason, fmt.Sprintf("Service account may not list %s; managed objects of these kinds are not deleted", strings.Join(kinds, ", ")))
}

// accessFailure reports RBAC denials of the Application service account as a
// non-retryable Forbidden failure and everything else with the given reason.
func accessFailure(err error, reason, fallback string, retryable bool) corev1alpha1.RevisionFailure {
	if apierrors.IsForbidden(err) {
		return corev1alpha1.RevisionFailure{Reason: "Forbidden", Message: safeMessage(err, "Application service account is not allowed to manage a resource"), Retryable: false}
	}
	return corev1alpha1.RevisionFailure{Reason: reason, Message: safeMessage(err, fallback), Retryable: retryable}
}

func (r *ApplicationReconciler) resolver() source.Resolver {
	if r.SourceResolver != nil {
		return r.SourceResolver
	}
	r.SourceResolver = gitcache.NewCache(cacheRoot(r.CacheDir))
	return r.SourceResolver
}

func (r *ApplicationReconciler) renderer(renderType corev1alpha1.RenderType) (renderer.Renderer, error) {
	if r.Renderers != nil {
		return r.Renderers(renderType)
	}
	switch renderType {
	case corev1alpha1.RenderTypeYAML:
		return yamlrenderer.Renderer{}, nil
	case corev1alpha1.RenderTypeKustomize:
		return kustomizerenderer.Renderer{}, nil
	case corev1alpha1.RenderTypeHelm:
		return helmrenderer.Renderer{}, nil
	default:
		return nil, fmt.Errorf("unsupported render type %q", renderType)
	}
}

// helmInputs are what rendering resolved from outside the Git checkout.
type helmInputs struct {
	chartDigest string
	// secretValues are Helm values read from Secrets; they are masked in
	// plans and render errors.
	secretValues []string
}

func (r *ApplicationReconciler) renderDesired(ctx context.Context, tenant client.Client, application *corev1alpha1.Application, resolved source.ResolvedSource) ([]unstructured.Unstructured, helmInputs, *corev1alpha1.RevisionFailure) {
	var inputs helmInputs
	desiredRenderer, err := r.renderer(application.Spec.Source.Render.Type)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "RenderFailure", Message: safeMessage(err, "Desired state renderer is not available"), Retryable: false}
		return nil, inputs, &failure
	}
	input := rendererInput(application, resolved.CacheDir)
	decryptor, err := r.decryptor(ctx, application)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "DecryptionFailure", Message: safeMessage(err, "Decryption keys could not be loaded"), Retryable: true}
		return nil, inputs, &failure
	}
	input.Decrypt = decryptor.File
	if helm := application.Spec.Source.Render.Helm; helm != nil && application.Spec.Source.Render.Type == corev1alpha1.RenderTypeHelm {
		if input.Values, inputs.secretValues, err = helmValues(ctx, tenant, application.Namespace, helm); err != nil {
			failure := accessFailure(err, "RenderFailure", "Helm values could not be read", true)
			return nil, inputs, &failure
		}
		if helm.Chart != nil {
			if input.ChartPath, inputs.chartDigest, err = r.pullChart(ctx, application.Namespace, helm.Chart); err != nil {
				failure := corev1alpha1.RevisionFailure{Reason: "RenderFailure", Message: safeMessage(err, "Helm chart could not be pulled"), Retryable: true}
				return nil, inputs, &failure
			}
		}
	}
	objects, err := desiredRenderer.Render(ctx, input)
	if err != nil {
		message := redactValues(safeMessage(err, "Desired state render failed"), inputs.secretValues)
		failure := corev1alpha1.RevisionFailure{Reason: "RenderFailure", Message: message, Retryable: true}
		return nil, inputs, &failure
	}
	return objects, inputs, nil
}

// pullChart downloads the Application's pinned chart with labelled
// registry credentials.
func (r *ApplicationReconciler) pullChart(ctx context.Context, namespace string, chart *corev1alpha1.HelmChartSource) (string, string, error) {
	src := helmrenderer.ChartSource{Repository: chart.Repository, Name: chart.Name, Version: chart.Version, CAFile: r.ChartCAFile}
	if chart.SecretRef != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: chart.SecretRef.Name}, secret); err != nil {
			return "", "", fmt.Errorf("chart repository Secret %s: %w", chart.SecretRef.Name, err)
		}
		if secret.Labels[RegistryCredentialsLabel] != "true" {
			return "", "", fmt.Errorf("chart repository Secret %s is not labelled %s=true", chart.SecretRef.Name, RegistryCredentialsLabel)
		}
		src.Username, src.Password = string(secret.Data["username"]), string(secret.Data["password"])
	}
	return helmrenderer.Pull(chartCacheDir(r.CacheDir), namespace, src)
}

// helmValues merges valuesFrom, in order, then inline values, reading
// ConfigMaps and Secrets as the Application's service account. It also
// returns every string value that came from a Secret.
func helmValues(ctx context.Context, tenant client.Client, namespace string, helm *corev1alpha1.HelmRenderSpec) (map[string]any, []string, error) {
	values := map[string]any{}
	secretValues := []string{}
	for _, ref := range helm.ValuesFrom {
		key := ref.Key
		if key == "" {
			key = "values.yaml"
		}
		var raw []byte
		switch ref.Kind {
		case "Secret":
			secret := &corev1.Secret{}
			if err := tenant.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, secret); err != nil {
				return nil, nil, err
			}
			raw = secret.Data[key]
		default:
			configMap := &corev1.ConfigMap{}
			if err := tenant.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, configMap); err != nil {
				return nil, nil, err
			}
			raw = []byte(configMap.Data[key])
		}
		if raw == nil {
			return nil, nil, fmt.Errorf("%s %s has no key %s", ref.Kind, ref.Name, key)
		}
		parsed := map[string]any{}
		if err := sigsyaml.Unmarshal(raw, &parsed); err != nil {
			return nil, nil, fmt.Errorf("%s %s key %s: %w", ref.Kind, ref.Name, key, err)
		}
		if ref.Kind == "Secret" {
			secretValues = append(secretValues, leafValues(parsed)...)
		}
		values = mergeValues(values, parsed)
	}
	if helm.Values != nil && len(helm.Values.Raw) > 0 {
		inline := map[string]any{}
		if err := json.Unmarshal(helm.Values.Raw, &inline); err != nil {
			return nil, nil, fmt.Errorf("inline Helm values: %w", err)
		}
		values = mergeValues(values, inline)
	}
	return values, secretValues, nil
}

// mergeValues deep-merges b over a, like Helm merges values files.
func mergeValues(a, b map[string]any) map[string]any {
	out := maps.Clone(a)
	if out == nil {
		out = map[string]any{}
	}
	for k, v := range b {
		if bm, ok := v.(map[string]any); ok {
			if am, ok := out[k].(map[string]any); ok {
				out[k] = mergeValues(am, bm)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func leafValues(value any) []string {
	switch typed := value.(type) {
	case map[string]any:
		out := make([]string, 0, len(typed))
		for _, v := range typed {
			out = append(out, leafValues(v)...)
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, v := range typed {
			out = append(out, leafValues(v)...)
		}
		return out
	case nil, bool:
		return nil
	default:
		return []string{fmt.Sprint(typed)}
	}
}

// redactValues masks secret values in text: values of four or more
// characters wherever they appear, shorter ones only as the whole text.
func redactValues(text string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) >= 4 {
			text = strings.ReplaceAll(text, secret, redact.Placeholder)
		} else if text == secret {
			text = redact.Placeholder
		}
	}
	return text
}

func redactPlanValues(plan *corev1alpha1.RevisionPlan, secrets []string) {
	if len(secrets) == 0 {
		return
	}
	for i := range plan.Resources {
		for j := range plan.Resources[i].Changes {
			change := &plan.Resources[i].Changes[j]
			before, after := redactValues(change.Before, secrets), redactValues(change.After, secrets)
			if before != change.Before || after != change.After {
				change.Before, change.After, change.Redacted = before, after, true
			}
		}
	}
}

func rendererInput(application *corev1alpha1.Application, workspace string) renderer.Input {
	input := renderer.Input{Workspace: workspace, Path: application.Spec.Source.Path, Namespace: application.DestinationNamespace()}
	if application.Spec.Source.Render.Helm != nil {
		input.ReleaseName = application.Spec.Source.Render.Helm.ReleaseName
		input.ValuesFiles = append([]string(nil), application.Spec.Source.Render.Helm.ValuesFiles...)
	}
	return input
}

func (r *ApplicationReconciler) ensureRevision(ctx context.Context, application *corev1alpha1.Application, revision string) (*corev1alpha1.Revision, error) {
	name, desiredHash, err := revisionIdentity(application, revision, effectiveServiceAccount(application, r.DefaultServiceAccount))
	if err != nil {
		return nil, err
	}
	out := &corev1alpha1.Revision{}
	key := client.ObjectKey{Namespace: application.Namespace, Name: name}
	if err := r.Get(ctx, key, out); err == nil {
		return out, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	out = &corev1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: application.Namespace,
			Labels: map[string]string{
				"solder.io/application": application.Name,
			},
		},
		Spec: corev1alpha1.RevisionSpec{
			ApplicationRef: corev1alpha1.LocalObjectReference{Name: application.Name},
			Source: corev1alpha1.RevisionSource{
				RepositoryRef: application.Spec.Source.RepositoryRef,
				Revision:      revision,
				Path:          application.Spec.Source.Path,
				Render:        application.Spec.Source.Render,
			},
			DesiredStateHash: desiredHash,
		},
	}
	if r.Scheme != nil {
		if err := controllerutil.SetControllerReference(application, out, r.Scheme); err != nil {
			return nil, err
		}
	}
	if err := r.Create(ctx, out); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return out, r.Get(ctx, key, out)
		}
		return nil, err
	}
	return out, nil
}

func desiredStateHash(objects []unstructured.Unstructured) (string, error) {
	ordered := append([]unstructured.Unstructured(nil), objects...)
	sort.Slice(ordered, func(i, j int) bool {
		left := ordered[i].GetAPIVersion() + "/" + ordered[i].GetKind() + "/" + ordered[i].GetNamespace() + "/" + ordered[i].GetName()
		right := ordered[j].GetAPIVersion() + "/" + ordered[j].GetKind() + "/" + ordered[j].GetNamespace() + "/" + ordered[j].GetName()
		return left < right
	})
	raw, err := json.Marshal(ordered)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// revisionIdentity includes the service account because who applies is part
// of a deployment attempt: switching to an account with the right permissions
// must start a fresh Revision instead of reusing one blocked by retry limits.
func revisionIdentity(application *corev1alpha1.Application, revision, serviceAccount string) (string, string, error) {
	raw, err := json.Marshal(struct {
		Application    string                            `json:"application"`
		Repository     corev1alpha1.LocalObjectReference `json:"repository"`
		Revision       string                            `json:"revision"`
		Path           string                            `json:"path"`
		Render         corev1alpha1.RenderSpec           `json:"render"`
		ServiceAccount string                            `json:"serviceAccount"`
	}{
		Application:    application.Name,
		Repository:     application.Spec.Source.RepositoryRef,
		Revision:       revision,
		Path:           application.Spec.Source.Path,
		Render:         application.Spec.Source.Render,
		ServiceAccount: serviceAccount,
	})
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s-%s", application.Name, hash[:12]), hash, nil
}

// markConflictPolicy records on each conflict how apply will treat it, so an
// adopting plan shows every field and manager it takes over.
func markConflictPolicy(plan *planner.Plan, policy corev1alpha1.ConflictPolicy) {
	for i := range plan.Changes {
		for j := range plan.Changes[i].Conflicts {
			plan.Changes[i].Conflicts[j].Policy = policy
		}
	}
}

func conflictFailure(plan planner.Plan) *corev1alpha1.RevisionFailure {
	for _, change := range plan.Changes {
		for _, conflict := range change.Conflicts {
			if conflict.Policy == corev1alpha1.ConflictPolicyFail {
				failure := corev1alpha1.RevisionFailure{Reason: "ConflictFailure", Message: fmt.Sprintf("server-side apply conflict on %s/%s %s/%s managed by %s", change.ID.APIVersion(), change.ID.Kind, change.ID.Namespace, change.ID.Name, conflict.Manager), Retryable: false}
				return &failure
			}
		}
	}
	return nil
}

func (r *ApplicationReconciler) updateRevisionStatus(ctx context.Context, revision *corev1alpha1.Revision) error {
	latest := &corev1alpha1.Revision{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(revision), latest); err != nil {
		return err
	}
	latest.Status = revision.Status
	if err := r.Status().Update(ctx, latest); err != nil {
		return err
	}
	revision.Status = latest.Status
	revision.ResourceVersion = latest.ResourceVersion
	return nil
}

func retryBlocked(application *corev1alpha1.Application, revision *corev1alpha1.Revision) *corev1alpha1.RevisionFailure {
	if revision.Status.Phase != corev1alpha1.RevisionPhaseFailed || revision.Status.Failure == nil {
		return nil
	}
	lastFailure := time.Time{}
	if revision.Status.CompletedAt != nil {
		lastFailure = revision.Status.CompletedAt.Time
	}
	// Back off exponentially from one second, capped at one minute.
	backoff := min(time.Second<<min(revision.Status.Attempts, 6), time.Minute)
	decision := retry.Decide(application.Spec.Strategy.FailurePolicy, retry.State{DesiredRevision: revision.Spec.Source.Revision, Attempts: revision.Status.Attempts, LastFailureAt: lastFailure}, time.Now(), backoff)
	if decision.Allowed {
		return nil
	}
	return &corev1alpha1.RevisionFailure{Reason: "RetryBlocked", Message: decision.Reason, Retryable: false}
}

// reportRetryBlocked records on the Application that retries stopped, keeping
// the Revision's original failure so operators still see why it failed.
func (r *ApplicationReconciler) reportRetryBlocked(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision, blocked corev1alpha1.RevisionFailure) error {
	message := blocked.Message
	if failure := revision.Status.Failure; failure != nil {
		message = fmt.Sprintf("%s; last failure %s: %s", blocked.Message, failure.Reason, failure.Message)
	}
	message = safeMessage(errors.New(message), blocked.Message)
	diagnosed := application.Status.Diagnosis
	r.markApplicationFailure(application, blocked.Reason, message)
	// Retries stopped after a failed health observation, whose diagnosis
	// still explains the Application.
	if failure := revision.Status.Failure; failure != nil && healthFailure(failure.Reason) {
		application.Status.Diagnosis = diagnosed
	}
	r.event(application, corev1.EventTypeWarning, blocked.Reason, message)
	return r.Status().Update(ctx, application)
}

func (r *ApplicationReconciler) reconcileDelete(ctx context.Context, application *corev1alpha1.Application) error {
	if !controllerutil.ContainsFinalizer(application, applicationFinalizer) {
		return nil
	}
	if application.Spec.DeletionPolicy == corev1alpha1.DeletionPolicyDeleteManagedResources {
		tenant, err := r.tenantClient(application)
		if err != nil {
			// Without a service account Solder may not delete anything, so the
			// managed resources are orphaned rather than blocking deletion forever.
			r.event(application, corev1.EventTypeWarning, "ManagedResourcesOrphaned", safeMessage(err, "Application service account could not be used"))
			controllerutil.RemoveFinalizer(application, applicationFinalizer)
			return r.Update(ctx, application)
		}
		managed, skipped, err := applier.ListManaged(ctx, tenant, application, applier.ListOptions{})
		if err != nil {
			return err
		}
		r.warnSkippedKinds(application, "ManagedResourcesOrphaned", skipped)
		plan := prune.Plan(managed, prune.Policy{Application: application.Name, AllowHighRisk: true})
		for _, obj := range ordering.Prune(plan.Eligible) {
			candidate := obj.DeepCopy()
			if err := tenant.Delete(ctx, candidate); client.IgnoreNotFound(err) != nil {
				if apierrors.IsForbidden(err) {
					r.event(application, corev1.EventTypeWarning, "Forbidden", safeMessage(err, "Application service account may not delete a managed resource"))
				}
				return err
			}
		}
	}
	controllerutil.RemoveFinalizer(application, applicationFinalizer)
	return r.Update(ctx, application)
}

func staleManagedObjects(desired, desiredLive, managed []unstructured.Unstructured) []unstructured.Unstructured {
	keep := map[resource.ID]struct{}{}
	for _, set := range [][]unstructured.Unstructured{desired, desiredLive} {
		for _, obj := range set {
			id, err := resource.FromObject(obj)
			if err == nil {
				keep[id] = struct{}{}
			}
		}
	}
	stale := []unstructured.Unstructured{}
	for _, obj := range managed {
		id, err := resource.FromObject(obj)
		if err != nil {
			continue
		}
		if _, ok := keep[id]; !ok {
			stale = append(stale, obj)
		}
	}
	return stale
}

func (r *ApplicationReconciler) applyAndObserve(ctx context.Context, tenant client.Client, application *corev1alpha1.Application, revision *corev1alpha1.Revision, desired, pruneCandidates []unstructured.Unstructured, progress rolloutProgress) (ctrl.Result, error) {
	// A retry after a failure is a new attempt; anything else in progress is
	// the same deployment carrying on.
	resuming := progress.state == rolloutInProgress && progress.previousPhase != corev1alpha1.RevisionPhaseFailed
	status.StartApplying(revision, application, metav1.Now())
	setRolloutComplete(revision, false)
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return ctrl.Result{}, err
	}
	if !resuming {
		r.event(application, corev1.EventTypeNormal, "DeploymentStarted", "Application deployment started")
	}
	evaluator, err := r.healthEvaluator(ctx, application)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "HealthFailure", Message: safeMessage(err, "HealthChecks could not be read"), Retryable: true}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	application.Status.ManagedKinds = managedKinds(objectKinds(desired))
	apply := applier.Applier{Client: tenant, ApplicationNamespace: application.Namespace}
	policy := syncpolicy.EffectiveConflictPolicy(application.Spec.Sync)
	results := []health.Result{}
	// observed holds the live managed objects health was evaluated on, for
	// diagnosis.
	observed := []unstructured.Unstructured{}
	pruned := false
	// Every reconcile walks the groups from the start: re-applying a group
	// that already converged is a no-op, and the walk stops at the first
	// group that is not yet Healthy. Hooks are the exception: each runs once
	// per rollout, so one already applied is only watched, never re-applied.
	for _, group := range ordering.Groups(desired) {
		if group.Stage == ordering.StagePostSync && !pruned {
			if failure := r.pruneStale(ctx, tenant, application, pruneCandidates); failure != nil {
				return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, *failure)
			}
			pruned = true
		}
		hook := group.Stage != ordering.StageSync
		pending, watched, done := group.Objects, []unstructured.Unstructured(nil), []health.Result(nil)
		if hook {
			pending, watched, done = splitHooks(group.Objects, progress.hooks)
			replacing, err := replaceStaleHooks(ctx, tenant, pending, revision.Name)
			if err != nil {
				failure := accessFailure(err, "HookFailed", "Previous hooks could not be removed", true)
				return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
			}
			if replacing {
				return r.observe(ctx, tenant, application, revision, results, observed, progress.previousHealth, 2*time.Second)
			}
		}
		if len(pending) > 0 {
			if err := apply.Apply(ctx, application.Name, revision.Name, pending, policy); err != nil {
				failure := accessFailure(err, "ApplyFailure", "Desired state apply failed", true)
				return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
			}
		}
		checked, read, err := groupHealth(ctx, tenant, evaluator, append(watched, pending...), hook)
		if err != nil {
			failure := accessFailure(err, "HealthFailure", "Applied resources could not be read", true)
			return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
		}
		if hook {
			recordHooks(revision, group.Stage, checked)
		}
		groupResults := append(done, checked...)
		results = append(results, groupResults...)
		observed = append(observed, read...)
		for _, result := range groupResults {
			if result.State != corev1alpha1.HealthStateDegraded {
				continue
			}
			failure := corev1alpha1.RevisionFailure{Reason: "HealthFailure", Message: "One or more resources are degraded", Retryable: true}
			if hook {
				failure = corev1alpha1.RevisionFailure{Reason: "HookFailed", Message: safeMessage(fmt.Errorf("%s hook %s/%s failed: %s", group.Stage, result.Resource.Kind, result.Resource.Name, result.Message), "A sync hook failed"), Retryable: true}
			}
			r.recordHealth(application, revision, results)
			r.diagnose(ctx, tenant, application, results, observed, true, progress.previousHealth)
			return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
		}
		if health.Summary(groupResults).Progressing > 0 {
			return r.observe(ctx, tenant, application, revision, results, observed, progress.previousHealth, 10*time.Second)
		}
	}
	if !pruned {
		if failure := r.pruneStale(ctx, tenant, application, pruneCandidates); failure != nil {
			return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, *failure)
		}
	}
	r.recordHealth(application, revision, results)
	if err := r.completeSuccessfulDeployment(ctx, application, revision, "Application deployment is healthy", true); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.applyHistoryRetention(ctx, application); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// observe records that the rollout is still progressing, and why, and checks
// back later, failing once the health timeout has passed.
func (r *ApplicationReconciler) observe(ctx context.Context, tenant client.Client, application *corev1alpha1.Application, revision *corev1alpha1.Revision, results []health.Result, observed []unstructured.Unstructured, previousHealth corev1alpha1.HealthState, after time.Duration) (ctrl.Result, error) {
	timedOut := application.Spec.Health.Timeout != nil && revision.Status.StartedAt != nil && time.Since(revision.Status.StartedAt.Time) > application.Spec.Health.Timeout.Duration
	r.diagnose(ctx, tenant, application, results, observed, timedOut, previousHealth)
	if timedOut {
		failure := corev1alpha1.RevisionFailure{Reason: "TimeoutFailure", Message: "Health observation timed out", Retryable: true}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	r.recordHealth(application, revision, results)
	revision.Status.Phase = corev1alpha1.RevisionPhaseObserving
	application.Status.Sync.State = corev1alpha1.SyncStateSynced
	application.Status.Health.State = corev1alpha1.HealthStateProgressing
	application.Status.State = corev1alpha1.HealthStateProgressing
	application.Status.DeployedRevision = revision.Spec.Source.Revision
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: after}, r.Status().Update(ctx, application)
}

func (r *ApplicationReconciler) recordHealth(application *corev1alpha1.Application, revision *corev1alpha1.Revision, results []health.Result) {
	summary := health.Summary(results)
	revision.Status.Health = summary
	application.Status.Resources = summary
}

// pruneStale deletes managed objects no longer in desired state.
func (r *ApplicationReconciler) pruneStale(ctx context.Context, tenant client.Client, application *corev1alpha1.Application, candidates []unstructured.Unstructured) *corev1alpha1.RevisionFailure {
	plan := prune.Plan(candidates, prune.Policy{Application: application.Name})
	if len(plan.Rejected) > 0 {
		return &corev1alpha1.RevisionFailure{Reason: "PruneFailure", Message: safeMessage(fmt.Errorf("%s", plan.Rejected[0].Reason), "Managed resource prune was rejected"), Retryable: false}
	}
	for _, obj := range ordering.Prune(plan.Eligible) {
		if err := tenant.Delete(ctx, obj.DeepCopy()); client.IgnoreNotFound(err) != nil {
			failure := accessFailure(err, "PruneFailure", "Managed resource prune failed", true)
			return &failure
		}
	}
	return nil
}

// groupHealth reads and evaluates the live state of one group's objects,
// all of which Solder has applied, and returns the live objects it read. A
// missing object is not there yet and is Progressing, except a hook: one
// that vanished before it was seen to succeed has failed, since it is never
// applied twice.
func groupHealth(ctx context.Context, tenant client.Client, evaluator health.Evaluator, objects []unstructured.Unstructured, hooks bool) ([]health.Result, []unstructured.Unstructured, error) {
	liveResult, err := (live.Reader{Client: tenant}).Read(ctx, objects)
	if err != nil {
		return nil, nil, err
	}
	found := ordering.Apply(slices.Collect(maps.Values(liveResult.Found)))
	results := make([]health.Result, 0, len(objects))
	for _, obj := range found {
		result, err := evaluator.Evaluate(obj)
		if err != nil {
			return nil, nil, err
		}
		results = append(results, result)
	}
	for _, id := range liveResult.Missing {
		result := health.Result{Resource: id, State: corev1alpha1.HealthStateProgressing, Reason: "NotFound", Message: "resource does not exist yet"}
		if hooks {
			result.State, result.Reason, result.Message = corev1alpha1.HealthStateDegraded, "HookMissing", "hook was deleted before it reported success"
		}
		results = append(results, result)
	}
	return results, found, nil
}

// replaceStaleHooks deletes hook objects left by an earlier Revision, so each
// Revision runs its hooks afresh, and reports whether any are still going.
// Hooks from the current Revision are kept until the next one replaces them.
func replaceStaleHooks(ctx context.Context, tenant client.Client, hooks []unstructured.Unstructured, revision string) (bool, error) {
	replacing := false
	for _, hook := range hooks {
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(hook.GroupVersionKind())
		if err := tenant.Get(ctx, client.ObjectKeyFromObject(&hook), current); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, err
		}
		if current.GetAnnotations()[applier.RevisionAnnotationKey] == revision {
			continue
		}
		replacing = true
		if current.GetDeletionTimestamp() == nil {
			if err := tenant.Delete(ctx, current, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				return false, err
			}
		}
	}
	return replacing, nil
}

func (r *ApplicationReconciler) completeSuccessfulDeployment(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision, healthyMessage string, transition bool) error {
	status.CompleteHealthy(revision, application, metav1.Now())
	application.Status.Diagnosis = nil
	setRolloutComplete(revision, true)
	if application.GetAnnotations()["solder.io/rollback-revision"] != "" {
		revision.Status.Phase = corev1alpha1.RevisionPhaseRolledBack
		application.Status.Sync.State = corev1alpha1.SyncStateOutOfSync
		annotations := application.GetAnnotations()
		delete(annotations, "solder.io/rollback-revision")
		application.SetAnnotations(annotations)
		if err := r.updateKeepingStatus(ctx, application); err != nil {
			return err
		}
		setReady(application, metav1.ConditionFalse, "RolledBack", "Application was rolled back to an earlier Revision after a failure")
		r.event(application, corev1.EventTypeNormal, "RollbackCompleted", "Application rollback completed")
		if transition {
			r.notify(ctx, application, revision, corev1alpha1.NotificationRolledBack, "Application rollback completed")
		}
	} else {
		setReady(application, metav1.ConditionTrue, "Healthy", "Application is Synced and Healthy")
		r.event(application, corev1.EventTypeNormal, "DeploymentHealthy", healthyMessage)
		if transition {
			r.notify(ctx, application, revision, corev1alpha1.NotificationHealthy, healthyMessage)
		}
	}
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return err
	}
	return r.Status().Update(ctx, application)
}

func (r *ApplicationReconciler) applyHistoryRetention(ctx context.Context, application *corev1alpha1.Application) error {
	var list corev1alpha1.RevisionList
	if err := r.List(ctx, &list, client.InNamespace(application.Namespace), client.MatchingLabels{"solder.io/application": application.Name}); err != nil {
		return err
	}
	retention := history.ApplyRetention(list.Items, history.LimitFor(*application, 20))
	for i := range retention.Delete {
		rev := retention.Delete[i]
		if err := r.Delete(ctx, &rev); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

// updateKeepingStatus updates the Application's metadata and spec, keeping the
// status this reconcile computed: Update replaces the object with the
// server's copy, whose status is the one persisted before this reconcile.
func (r *ApplicationReconciler) updateKeepingStatus(ctx context.Context, application *corev1alpha1.Application) error {
	computed := application.Status.DeepCopy()
	err := r.Update(ctx, application)
	application.Status = *computed
	return err
}

// healthFailure reports failure reasons decided by observing the health of
// managed objects, which status.diagnosis explains.
func healthFailure(reason string) bool {
	switch reason {
	case "HealthFailure", "HookFailed", "TimeoutFailure":
		return true
	}
	return false
}

func (r *ApplicationReconciler) failRevisionAndApplication(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision, failure corev1alpha1.RevisionFailure) error {
	now := metav1.Now()
	rollbackQueued := false
	rollbackMissing := false
	if application.Spec.Strategy.FailurePolicy.Action == corev1alpha1.FailureActionRollback && application.GetAnnotations()["solder.io/rollback-revision"] == "" {
		if target, err := r.rollbackTarget(ctx, application, revision); err == nil {
			metav1.SetMetaDataAnnotation(&application.ObjectMeta, "solder.io/rollback-revision", target.Spec.Source.Revision)
			revision.Status.PreviousRevision = &corev1alpha1.LocalObjectReference{Name: target.Name}
			rollbackQueued = true
			if err := r.updateKeepingStatus(ctx, application); err != nil {
				return err
			}
		} else {
			rollbackMissing = true
		}
	}
	status.Fail(revision, application, now, failure)
	setReady(application, metav1.ConditionFalse, failure.Reason, failure.Message)
	if !healthFailure(failure.Reason) {
		application.Status.Diagnosis = nil
	}
	if rollbackQueued {
		revision.Status.Phase = corev1alpha1.RevisionPhaseRollingBack
		r.event(application, corev1.EventTypeWarning, "RollbackStarted", "Application failure triggered rollback")
	} else if rollbackMissing {
		revision.Status.Failure = &corev1alpha1.RevisionFailure{Reason: "RollbackFailed", Message: "No previous healthy Revision is available for rollback", Retryable: false}
	}
	r.event(application, corev1.EventTypeWarning, failure.Reason, failure.Message)
	r.notify(ctx, application, revision, corev1alpha1.NotificationFailed, failure.Reason+": "+failure.Message)
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return err
	}
	return r.Status().Update(ctx, application)
}

func (r *ApplicationReconciler) rollbackTarget(ctx context.Context, application *corev1alpha1.Application, current *corev1alpha1.Revision) (corev1alpha1.Revision, error) {
	var list corev1alpha1.RevisionList
	if err := r.List(ctx, &list, client.InNamespace(application.Namespace), client.MatchingLabels{"solder.io/application": application.Name}); err != nil {
		return corev1alpha1.Revision{}, err
	}
	return rollback.Target(*current, list.Items)
}

// validateHelmReleaseName returns an error when a Helm Application's release
// name, or the default used in its place, breaks Helm's naming rule.
func validateHelmReleaseName(application *corev1alpha1.Application) error {
	render := application.Spec.Source.Render
	if render.Type != corev1alpha1.RenderTypeHelm {
		return nil
	}
	name := ""
	if render.Helm != nil {
		name = render.Helm.ReleaseName
	}
	name = helmrenderer.ReleaseName(name)
	if err := chartutil.ValidateReleaseName(name); err != nil {
		return fmt.Errorf("helm release name %q is not valid: it must be a lowercase DNS subdomain of at most 53 characters; rename the release", name)
	}
	return nil
}

// markApplicationFailure records a failure that stopped reconciliation
// before health was observed, clearing a diagnosis that no longer explains
// the Application.
func (r *ApplicationReconciler) markApplicationFailure(application *corev1alpha1.Application, reason, message string) {
	application.Status.Diagnosis = nil
	application.Status.ObservedGeneration = application.Generation
	application.Status.State = corev1alpha1.HealthStateDegraded
	application.Status.Health.State = corev1alpha1.HealthStateDegraded
	application.Status.Sync.State = corev1alpha1.SyncStateOutOfSync
	setReady(application, metav1.ConditionFalse, reason, message)
}

// ReadyCondition is the Application condition that is True only while the
// Application is Synced to its desired Revision and Healthy.
const ReadyCondition = "Ready"

// setReady records the Ready condition. Its transition time moves only when
// the status flips, and a fixed message per reason keeps steady-state
// reconciles from rewriting it.
func setReady(application *corev1alpha1.Application, state metav1.ConditionStatus, reason, message string) {
	apimeta.SetStatusCondition(&application.Status.Conditions, metav1.Condition{
		Type:               ReadyCondition,
		Status:             state,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: application.Generation,
	})
}

func (r *ApplicationReconciler) event(application *corev1alpha1.Application, eventType, reason, message string) {
	ops.ObserveLifecycleEvent(*application, "", reason)
	if r.Recorder == nil {
		return
	}
	r.Recorder.Event(application, eventType, reason, redact.String(message))
}

func (r *ApplicationReconciler) planLimit() int {
	if r.PlanLimit > 0 {
		return r.PlanLimit
	}
	return defaultPlanResourceLimit
}

// defaultDestinationNamespace places namespaced objects without a namespace in
// the destination namespace, leaving cluster-scoped objects untouched.
func defaultDestinationNamespace(objects []unstructured.Unstructured, namespace string, scopes resource.Scopes) error {
	if namespace == "" {
		return nil
	}
	for i := range objects {
		if objects[i].GetNamespace() != "" {
			continue
		}
		namespaced, err := scopes.Namespaced(objects[i])
		if err != nil {
			return err
		}
		if namespaced {
			objects[i].SetNamespace(namespace)
		}
	}
	return nil
}

func failureReason(err error, fallback string) string {
	var sourceErr *source.Error
	if errors.As(err, &sourceErr) && sourceErr.Reason != "" {
		return string(sourceErr.Reason)
	}
	return fallback
}

func safeMessage(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	message := redact.String(err.Error())
	if message == "" {
		message = fallback
	}
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}

func (r *ApplicationReconciler) applicationsForRepository(ctx context.Context, obj client.Object) []reconcile.Request {
	var apps corev1alpha1.ApplicationList
	if err := r.List(ctx, &apps, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	requests := []reconcile.Request{}
	for _, app := range apps.Items {
		if app.Spec.Source.RepositoryRef.Name == obj.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&app)})
		}
	}
	return requests
}

func managedObjectToApplication(_ context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()
	name := labels[applier.ApplicationLabelKey]
	if name == "" {
		return nil
	}
	namespace := labels[applier.ApplicationNamespaceLabelKey]
	if namespace == "" {
		namespace = obj.GetNamespace()
	}
	return []reconcile.Request{{NamespacedName: client.ObjectKey{Namespace: namespace, Name: name}}}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		// The deprecated core recorder stays: the events.k8s.io recorder merges
		// events that differ only in message; see .procoder/todo.
		r.Recorder = mgr.GetEventRecorderFor("application-controller")
	}
	built, err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Application{}).
		Watches(&corev1alpha1.Application{}, handler.EnqueueRequestsFromMapFunc(r.dependentsOf)).
		Watches(&corev1alpha1.Repository{}, handler.EnqueueRequestsFromMapFunc(r.applicationsForRepository)).
		WatchesMetadata(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		WatchesMetadata(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		WatchesMetadata(&corev1.Service{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		WatchesMetadata(&appsv1.Deployment{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		WatchesMetadata(&appsv1.StatefulSet{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		WatchesMetadata(&appsv1.DaemonSet{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		Named("application").
		Build(r)
	if err != nil {
		return err
	}
	recheck := r.DriftResyncInterval
	if recheck <= 0 {
		recheck = 5 * time.Minute
	}
	r.watches = newDriftWatches(built, mgr.GetCache(), r.Client, recheck)
	return nil
}

// notify queues a lifecycle notification for every sink subscribed to the
// event and records whether all subscribed sinks are usable. Misconfigured or
// failing sinks never block or fail reconciliation.
func (r *ApplicationReconciler) notify(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision, event corev1alpha1.NotificationEvent, message string) {
	if len(application.Spec.Notifications) == 0 {
		return
	}
	condition := metav1.Condition{Type: "NotificationsReady", Status: metav1.ConditionTrue, Reason: "SinksReady", Message: "Notification sinks are configured", ObservedGeneration: application.Generation}
	for _, subscription := range application.Spec.Notifications {
		target, err := r.notificationTarget(ctx, application.Namespace, subscription.SinkRef.Name)
		if err != nil {
			condition.Status, condition.Reason = metav1.ConditionFalse, "SinkInvalid"
			condition.Message = safeMessage(err, "A notification sink is invalid")
			continue
		}
		if r.Notifier == nil || !slices.Contains(subscription.Events, event) {
			continue
		}
		msg := notify.Message{
			Event: event, Application: application.Name, Namespace: application.Namespace,
			Revision: revision.Name, SourceRevision: revision.Spec.Source.Revision,
			Message: redact.String(message), Plan: revision.Status.Plan.Summary, Time: time.Now().UTC(),
		}
		if event == corev1alpha1.NotificationAwaitingApproval {
			msg.ApproveCommand = fmt.Sprintf("solder approve %s -n %s --revision %s", application.Name, application.Namespace, revision.Name)
		}
		failed := application.DeepCopy()
		sink := subscription.SinkRef.Name
		if !r.Notifier.Enqueue(notify.Delivery{Target: target, Message: msg, OnFailure: func(err error) {
			r.event(failed, corev1.EventTypeWarning, "NotificationFailed", fmt.Sprintf("Notification to sink %s failed: %s", sink, safeMessage(err, "delivery failed")))
		}}) {
			r.event(application, corev1.EventTypeWarning, "NotificationDropped", fmt.Sprintf("Notification to sink %s was dropped: the queue is full", sink))
		}
	}
	apimeta.SetStatusCondition(&application.Status.Conditions, condition)
}

// notificationTarget resolves a NotificationSink and its Secret.
func (r *ApplicationReconciler) notificationTarget(ctx context.Context, namespace, name string) (notify.Target, error) {
	sink := &corev1alpha1.NotificationSink{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, sink); err != nil {
		return notify.Target{}, fmt.Errorf("NotificationSink %s: %w", name, err)
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: sink.Spec.SecretRef.Name}, secret); err != nil {
		return notify.Target{}, fmt.Errorf("NotificationSink %s secret: %w", name, err)
	}
	target := notify.Target{Type: sink.Spec.Type, URL: string(secret.Data["url"]), HMACKey: string(secret.Data["hmacKey"])}
	parsed, err := url.Parse(target.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return notify.Target{}, fmt.Errorf("NotificationSink %s secret must hold an https url", name)
	}
	if target.Type == corev1alpha1.NotificationSinkWebhook && target.HMACKey == "" {
		return notify.Target{}, fmt.Errorf("NotificationSink %s secret must hold an hmacKey for webhook signing", name)
	}
	return target, nil
}

// dependenciesReady reports whether every dependsOn Application is Healthy at
// its desired revision, recording the DependenciesReady condition. A cycle is
// reported and never becomes ready; dependents are re-queued by a watch
// instead of polling.
func (r *ApplicationReconciler) dependenciesReady(ctx context.Context, application *corev1alpha1.Application) (bool, error) {
	if len(application.Spec.DependsOn) == 0 {
		apimeta.RemoveStatusCondition(&application.Status.Conditions, "DependenciesReady")
		return true, nil
	}
	var apps corev1alpha1.ApplicationList
	if err := r.List(ctx, &apps, client.InNamespace(application.Namespace)); err != nil {
		return false, err
	}
	byName := map[string]*corev1alpha1.Application{}
	for i := range apps.Items {
		byName[apps.Items[i].Name] = &apps.Items[i]
	}
	condition := metav1.Condition{Type: "DependenciesReady", Status: metav1.ConditionTrue, Reason: "DependenciesHealthy", Message: "All dependencies are Healthy", ObservedGeneration: application.Generation}
	if cycle := dependencyCycle(application.Name, application.Spec.DependsOn, byName); cycle != "" {
		condition.Status, condition.Reason, condition.Message = metav1.ConditionFalse, "DependencyCycle", "dependsOn forms a cycle: "+cycle
	} else {
		pending := []string{}
		for _, dep := range application.Spec.DependsOn {
			if !dependencyHealthy(byName[dep.Name]) {
				pending = append(pending, dep.Name)
			}
		}
		if len(pending) > 0 {
			condition.Status, condition.Reason = metav1.ConditionFalse, "DependencyNotReady"
			condition.Message = "Waiting for Healthy dependencies: " + strings.Join(pending, ", ")
		}
	}
	apimeta.SetStatusCondition(&application.Status.Conditions, condition)
	return condition.Status == metav1.ConditionTrue, nil
}

func dependencyHealthy(dep *corev1alpha1.Application) bool {
	return dep != nil &&
		dep.Status.ObservedGeneration == dep.Generation &&
		dep.Status.Health.State == corev1alpha1.HealthStateHealthy &&
		dep.Status.DesiredRevision != "" &&
		dep.Status.DeployedRevision == dep.Status.DesiredRevision
}

// dependencyCycle returns the cycle through start as "a -> b -> a", or "".
func dependencyCycle(start string, deps []corev1alpha1.LocalObjectReference, byName map[string]*corev1alpha1.Application) string {
	var visit func(name string, path []string, seen map[string]bool) []string
	visit = func(name string, path []string, seen map[string]bool) []string {
		if name == start && len(path) > 0 {
			return append(path, name)
		}
		if seen[name] {
			return nil
		}
		seen[name] = true
		next := deps
		if len(path) > 0 {
			app := byName[name]
			if app == nil {
				return nil
			}
			next = app.Spec.DependsOn
		}
		for _, dep := range next {
			if cycle := visit(dep.Name, append(path, name), seen); cycle != nil {
				return cycle
			}
		}
		return nil
	}
	if cycle := visit(start, nil, map[string]bool{}); cycle != nil {
		return strings.Join(cycle, " -> ")
	}
	return ""
}

// dependentsOf re-queues Applications in the same namespace that depend on
// the changed Application.
func (r *ApplicationReconciler) dependentsOf(ctx context.Context, obj client.Object) []reconcile.Request {
	var apps corev1alpha1.ApplicationList
	if err := r.List(ctx, &apps, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	requests := []reconcile.Request{}
	for _, app := range apps.Items {
		for _, dep := range app.Spec.DependsOn {
			if dep.Name == obj.GetName() {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&app)})
				break
			}
		}
	}
	return requests
}

// DecryptionKeyLabel marks a Secret that Solder may use for decryption keys.
const DecryptionKeyLabel = "solder.io/decryption-key"

// decryptor loads the Application's age keys. Without spec.decryption it
// returns nil, which refuses encrypted files rather than applying ciphertext.
func (r *ApplicationReconciler) decryptor(ctx context.Context, application *corev1alpha1.Application) (*decrypt.Decryptor, error) {
	spec := application.Spec.Decryption
	if spec == nil {
		return nil, nil
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: application.Namespace, Name: spec.SecretRef.Name}, secret); err != nil {
		return nil, fmt.Errorf("decryption Secret %s: %w", spec.SecretRef.Name, err)
	}
	if secret.Labels[DecryptionKeyLabel] != "true" {
		return nil, fmt.Errorf("decryption Secret %s is not labelled %s=true", spec.SecretRef.Name, DecryptionKeyLabel)
	}
	keys := []string{}
	for name, value := range secret.Data {
		if strings.HasSuffix(name, ".agekey") {
			keys = append(keys, string(value))
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("decryption Secret %s has no .agekey entries", spec.SecretRef.Name)
	}
	return decrypt.New(keys...)
}
