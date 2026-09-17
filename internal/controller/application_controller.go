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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/applier"
	"github.com/azrtydxb/solder/internal/health"
	"github.com/azrtydxb/solder/internal/history"
	"github.com/azrtydxb/solder/internal/live"
	"github.com/azrtydxb/solder/internal/ops"
	"github.com/azrtydxb/solder/internal/ordering"
	"github.com/azrtydxb/solder/internal/planner"
	"github.com/azrtydxb/solder/internal/prune"
	"github.com/azrtydxb/solder/internal/redact"
	"github.com/azrtydxb/solder/internal/renderer"
	execrenderer "github.com/azrtydxb/solder/internal/renderer/exec"
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
	Metrics        ops.ApplicationMetrics
}

// +kubebuilder:rbac:groups=solder.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solder.io,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solder.io,resources=applications/finalizers,verbs=update
// +kubebuilder:rbac:groups=solder.io,resources=repositories,verbs=get;list;watch
// +kubebuilder:rbac:groups=solder.io,resources=revisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solder.io,resources=revisions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get
// +kubebuilder:rbac:groups="",resources=configmaps;services;secrets;persistentvolumeclaims;serviceaccounts;namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;daemonsets;replicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs;cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete

// Reconcile resolves, renders, validates, plans, applies approved changes, and observes health.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reconcileErr error) {
	if r.Tracer != nil {
		var finish func(error)
		ctx, finish = r.Tracer.Start(ctx, "Application/Reconcile")
		defer func() { finish(reconcileErr) }()
	}
	metricApp := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: req.Namespace}}
	metricPhase := corev1alpha1.RevisionPhase("")
	metrics := r.Metrics
	if metrics == nil {
		metrics = ops.PrometheusApplicationMetrics()
	}
	defer func() { ops.ObserveApplicationReconcile(metrics, metricApp, metricPhase, reconcileErr) }()
	log := logf.FromContext(ctx)

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
	if blocked := retryBlocked(application, revision); blocked != nil {
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, *blocked)
	}

	now := metav1.Now()
	if revision.Status.Phase == "" || revision.Status.Phase == corev1alpha1.RevisionPhasePending || revision.Status.Phase == corev1alpha1.RevisionPhaseFailed {
		revision.Status.Attempts++
	}
	status.StartPlanning(revision, application, now)
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return ctrl.Result{}, err
	}

	rendered, failure := r.renderDesired(ctx, application, resolved)
	if failure != nil {
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, *failure)
	}
	defaultDestinationNamespace(rendered, application.Spec.Destination.Namespace)
	if desiredHash, err := desiredStateHash(rendered); err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "PlanFailure", Message: safeMessage(err, "Desired state could not be fingerprinted"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	} else if desiredHash != revision.Spec.DesiredStateHash {
		revision.Spec.DesiredStateHash = desiredHash
		if err := r.Update(ctx, revision); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := validate.Desired(rendered, validate.Options{DestinationNamespace: application.Spec.Destination.Namespace}); err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "ValidationFailure", Message: safeMessage(err, "Rendered desired state is invalid"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}

	liveResult, err := (live.Reader{Client: r.Client}).Read(ctx, rendered)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "PlanFailure", Message: safeMessage(err, "Live state could not be read"), Retryable: true}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	liveObjects := make([]unstructured.Unstructured, 0, len(liveResult.Found))
	for _, obj := range liveResult.Found {
		liveObjects = append(liveObjects, obj)
	}
	managedStale := []unstructured.Unstructured{}
	if application.Spec.Sync.Prune {
		managed, err := r.listManagedObjects(ctx, application)
		if err != nil {
			failure := corev1alpha1.RevisionFailure{Reason: "PlanFailure", Message: safeMessage(err, "Managed resources could not be inventoried"), Retryable: true}
			return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
		}
		managedStale = staleManagedObjects(rendered, liveObjects, managed)
		liveObjects = append(liveObjects, managedStale...)
	}

	plan, err := planner.Build(rendered, liveObjects)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "PlanFailure", Message: safeMessage(err, "Plan could not be built"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	revision.Status.Plan = plan.RevisionPlan(r.planLimit())
	revision.Status.Failure = nil
	r.event(application, corev1.EventTypeNormal, "PlanCreated", "Application plan created")
	if failure := conflictFailure(plan); failure != nil {
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, *failure)
	}
	if plan.Summary.Create == 0 && plan.Summary.Update == 0 && plan.Summary.Delete == 0 {
		if err := r.completeSuccessfulDeployment(ctx, application, revision, "Application already synced"); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Application already synced", "application", application.Name, "namespace", application.Namespace, "revision", resolved.Revision, "revisionRecord", revision.Name)
		return ctrl.Result{}, nil
	}
	if application.Status.DeployedRevision == resolved.Revision && !application.Spec.Sync.SelfHeal {
		application.Status.Sync.State = corev1alpha1.SyncStateDrifted
		revision.Status.Phase = corev1alpha1.RevisionPhaseObserving
		if err := r.updateRevisionStatus(ctx, revision); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Status().Update(ctx, application); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if !r.shouldApply(application, revision) {
		status.AwaitApproval(revision, application)
		if err := r.updateRevisionStatus(ctx, revision); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Status().Update(ctx, application); err != nil {
			return ctrl.Result{}, err
		}
		r.event(application, corev1.EventTypeNormal, "ApprovalRequired", "Application plan is awaiting approval")
		log.Info("Planned Application", "application", application.Name, "namespace", application.Namespace, "revision", resolved.Revision, "revisionRecord", revision.Name)
		return ctrl.Result{}, nil
	}
	return r.applyAndObserve(ctx, application, revision, rendered, managedStale)
}

func (r *ApplicationReconciler) resolver() source.Resolver {
	if r.SourceResolver != nil {
		return r.SourceResolver
	}
	root := r.CacheDir
	if root == "" {
		root = filepath.Join(os.TempDir(), defaultSourceCacheDir)
	}
	r.SourceResolver = gitcache.NewCache(root)
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
		return execrenderer.KustomizeRenderer{}, nil
	case corev1alpha1.RenderTypeHelm:
		return execrenderer.HelmRenderer{}, nil
	default:
		return nil, fmt.Errorf("unsupported render type %q", renderType)
	}
}

func (r *ApplicationReconciler) renderDesired(ctx context.Context, application *corev1alpha1.Application, resolved source.ResolvedSource) ([]unstructured.Unstructured, *corev1alpha1.RevisionFailure) {
	renderer, err := r.renderer(application.Spec.Source.Render.Type)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "RenderFailure", Message: safeMessage(err, "Desired state renderer is not available"), Retryable: false}
		return nil, &failure
	}
	input := rendererInput(application, resolved.CacheDir)
	objects, err := renderer.Render(ctx, input)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "RenderFailure", Message: safeMessage(err, "Desired state render failed"), Retryable: true}
		return nil, &failure
	}
	return objects, nil
}

func rendererInput(application *corev1alpha1.Application, workspace string) renderer.Input {
	input := renderer.Input{Workspace: workspace, Path: application.Spec.Source.Path}
	if application.Spec.Source.Render.Helm != nil {
		input.ReleaseName = application.Spec.Source.Render.Helm.ReleaseName
		input.ValuesFiles = append([]string(nil), application.Spec.Source.Render.Helm.ValuesFiles...)
	}
	return input
}

func (r *ApplicationReconciler) ensureRevision(ctx context.Context, application *corev1alpha1.Application, revision string) (*corev1alpha1.Revision, error) {
	name, desiredHash, err := revisionIdentity(application, revision)
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

func revisionIdentity(application *corev1alpha1.Application, revision string) (string, string, error) {
	raw, err := json.Marshal(struct {
		Application string                            `json:"application"`
		Repository  corev1alpha1.LocalObjectReference `json:"repository"`
		Revision    string                            `json:"revision"`
		Path        string                            `json:"path"`
		Render      corev1alpha1.RenderSpec           `json:"render"`
	}{
		Application: application.Name,
		Repository:  application.Spec.Source.RepositoryRef,
		Revision:    revision,
		Path:        application.Spec.Source.Path,
		Render:      application.Spec.Source.Render,
	})
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s-%s", application.Name, hash[:12]), hash, nil
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
	decision := retry.Decide(application.Spec.Strategy.FailurePolicy, retry.State{DesiredRevision: revision.Spec.Source.Revision, DeployedRevision: application.Status.DeployedRevision, Attempts: revision.Status.Attempts, LastFailureAt: lastFailure, Suspended: application.Spec.Suspend}, time.Now(), ops.RateLimiter{Base: time.Second, Max: time.Minute}.Delay(int(revision.Status.Attempts)))
	if decision.Allowed {
		return nil
	}
	return &corev1alpha1.RevisionFailure{Reason: "RetryBlocked", Message: decision.Reason, Retryable: false}
}

func (r *ApplicationReconciler) shouldApply(application *corev1alpha1.Application, revision *corev1alpha1.Revision) bool {
	if application.Spec.Sync.Automatic {
		return true
	}
	return application.GetAnnotations()["solder.io/approved-revision"] == revision.Name
}

func (r *ApplicationReconciler) reconcileDelete(ctx context.Context, application *corev1alpha1.Application) error {
	if !controllerutil.ContainsFinalizer(application, applicationFinalizer) {
		return nil
	}
	if application.Spec.DeletionPolicy == corev1alpha1.DeletionPolicyDeleteManagedResources {
		managed, err := r.listManagedObjects(ctx, application)
		if err != nil {
			return err
		}
		plan := prune.Plan(managed, prune.Policy{Application: application.Name, AllowHighRisk: true})
		for _, obj := range ordering.Prune(plan.Eligible) {
			candidate := obj.DeepCopy()
			if err := r.Delete(ctx, candidate); client.IgnoreNotFound(err) != nil {
				return err
			}
		}
	}
	controllerutil.RemoveFinalizer(application, applicationFinalizer)
	return r.Update(ctx, application)
}

func (r *ApplicationReconciler) listManagedObjects(ctx context.Context, application *corev1alpha1.Application) ([]unstructured.Unstructured, error) {
	gvks := []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "ConfigMap"},
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "", Version: "v1", Kind: "Service"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		{Group: "apps", Version: "v1", Kind: "DaemonSet"},
	}
	out := []unstructured.Unstructured{}
	for _, gvk := range gvks {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind + "List"})
		namespace := application.Spec.Destination.Namespace
		if namespace == "" {
			namespace = application.Namespace
		}
		if err := r.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{applier.ApplicationLabelKey: application.Name}); err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			ownerNamespace := item.GetLabels()[applier.ApplicationNamespaceLabelKey]
			if ownerNamespace == "" || ownerNamespace == application.Namespace {
				out = append(out, item)
			}
		}
	}
	return out, nil
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

func (r *ApplicationReconciler) applyAndObserve(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision, desired, pruneCandidates []unstructured.Unstructured) (ctrl.Result, error) {
	status.StartApplying(revision, application, metav1.Now())
	if err := syncpolicy.EnsureMutationAllowed(*application, revision.Status.Phase); err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "ApplyBlocked", Message: safeMessage(err, "Application is not allowed to apply"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return ctrl.Result{}, err
	}
	r.event(application, corev1.EventTypeNormal, "DeploymentStarted", "Application deployment started")
	ordered := ordering.Apply(desired)
	_, err := (applier.Applier{Client: r.Client, ApplicationNamespace: application.Namespace}).Apply(ctx, application.Name, revision.Name, ordered, syncpolicy.EffectiveConflictPolicy(application.Spec.Sync))
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "ApplyFailure", Message: safeMessage(err, "Desired state apply failed"), Retryable: true}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	prunePlan := prune.Plan(pruneCandidates, prune.Policy{Application: application.Name})
	if len(prunePlan.Rejected) > 0 {
		failure := corev1alpha1.RevisionFailure{Reason: "PruneFailure", Message: safeMessage(fmt.Errorf("%s", prunePlan.Rejected[0].Reason), "Managed resource prune was rejected"), Retryable: false}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	for _, obj := range ordering.Prune(prunePlan.Eligible) {
		candidate := obj.DeepCopy()
		if err := r.Delete(ctx, candidate); client.IgnoreNotFound(err) != nil {
			failure := corev1alpha1.RevisionFailure{Reason: "PruneFailure", Message: safeMessage(err, "Managed resource prune failed"), Retryable: true}
			return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
		}
	}
	liveResult, err := (live.Reader{Client: r.Client}).Read(ctx, desired)
	if err != nil {
		failure := corev1alpha1.RevisionFailure{Reason: "HealthFailure", Message: safeMessage(err, "Applied resources could not be read"), Retryable: true}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	liveObjects := make([]unstructured.Unstructured, 0, len(liveResult.Found))
	for _, obj := range liveResult.Found {
		liveObjects = append(liveObjects, obj)
	}
	healthResults := make([]health.Result, 0, len(liveObjects))
	for _, obj := range liveObjects {
		result, err := health.Evaluate(obj)
		if err != nil {
			failure := corev1alpha1.RevisionFailure{Reason: "HealthFailure", Message: safeMessage(err, "Resource health could not be evaluated"), Retryable: true}
			return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
		}
		healthResults = append(healthResults, result)
	}
	summary := health.Summary(healthResults)
	revision.Status.Health = summary
	application.Status.Resources = summary
	if summary.Degraded > 0 {
		failure := corev1alpha1.RevisionFailure{Reason: "HealthFailure", Message: "One or more resources are degraded", Retryable: true}
		return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
	}
	if summary.Progressing > 0 || summary.Unknown > 0 {
		if application.Spec.Health.Timeout != nil && revision.Status.StartedAt != nil && time.Since(revision.Status.StartedAt.Time) > application.Spec.Health.Timeout.Duration {
			failure := corev1alpha1.RevisionFailure{Reason: "TimeoutFailure", Message: "Health observation timed out", Retryable: true}
			return ctrl.Result{}, r.failRevisionAndApplication(ctx, application, revision, failure)
		}
		revision.Status.Phase = corev1alpha1.RevisionPhaseObserving
		application.Status.Sync.State = corev1alpha1.SyncStateSynced
		application.Status.Health.State = corev1alpha1.HealthStateProgressing
		application.Status.State = corev1alpha1.HealthStateProgressing
		application.Status.DeployedRevision = revision.Spec.Source.Revision
		if err := r.updateRevisionStatus(ctx, revision); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, application)
	}
	if err := r.completeSuccessfulDeployment(ctx, application, revision, "Application deployment is healthy"); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.applyHistoryRetention(ctx, application); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ApplicationReconciler) completeSuccessfulDeployment(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision, healthyMessage string) error {
	status.CompleteHealthy(revision, application, metav1.Now())
	if application.GetAnnotations()["solder.io/rollback-revision"] != "" {
		revision.Status.Phase = corev1alpha1.RevisionPhaseRolledBack
		application.Status.Sync.State = corev1alpha1.SyncStateOutOfSync
		annotations := application.GetAnnotations()
		delete(annotations, "solder.io/rollback-revision")
		application.SetAnnotations(annotations)
		if err := r.Update(ctx, application); err != nil {
			return err
		}
		r.event(application, corev1.EventTypeNormal, "RollbackCompleted", "Application rollback completed")
	} else {
		r.event(application, corev1.EventTypeNormal, "DeploymentHealthy", healthyMessage)
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

func (r *ApplicationReconciler) failRevisionAndApplication(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision, failure corev1alpha1.RevisionFailure) error {
	now := metav1.Now()
	rollbackQueued := false
	rollbackMissing := false
	if application.Spec.Strategy.FailurePolicy.Action == corev1alpha1.FailureActionRollback && application.GetAnnotations()["solder.io/rollback-revision"] == "" {
		if target, err := r.rollbackTarget(ctx, application, revision); err == nil {
			annotations := application.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations["solder.io/rollback-revision"] = target.Spec.Source.Revision
			application.SetAnnotations(annotations)
			revision.Status.PreviousRevision = &corev1alpha1.LocalObjectReference{Name: target.Name}
			rollbackQueued = true
			if err := r.Update(ctx, application); err != nil {
				return err
			}
		} else {
			rollbackMissing = true
		}
	}
	status.Fail(revision, application, now, failure)
	if rollbackQueued {
		revision.Status.Phase = corev1alpha1.RevisionPhaseRollingBack
		r.event(application, corev1.EventTypeWarning, "RollbackStarted", "Application failure triggered rollback")
	} else if rollbackMissing {
		revision.Status.Failure = &corev1alpha1.RevisionFailure{Reason: "RollbackFailed", Message: "No previous healthy Revision is available for rollback", Retryable: false}
	}
	r.event(application, corev1.EventTypeWarning, failure.Reason, failure.Message)
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

func (r *ApplicationReconciler) markApplicationFailure(application *corev1alpha1.Application, reason, message string) {
	application.Status.ObservedGeneration = application.Generation
	application.Status.State = corev1alpha1.HealthStateDegraded
	application.Status.Health.State = corev1alpha1.HealthStateDegraded
	application.Status.Sync.State = corev1alpha1.SyncStateOutOfSync
	apimeta.SetStatusCondition(&application.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
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

func defaultDestinationNamespace(objects []unstructured.Unstructured, namespace string) {
	if namespace == "" {
		return
	}
	for i := range objects {
		if objects[i].GetNamespace() == "" && !isKnownClusterScoped(objects[i]) {
			objects[i].SetNamespace(namespace)
		}
	}
}

func isKnownClusterScoped(obj unstructured.Unstructured) bool {
	if obj.GetAPIVersion() == "v1" {
		switch obj.GetKind() {
		case "Namespace", "Node", "PersistentVolume":
			return true
		}
	}
	if strings.HasPrefix(obj.GetAPIVersion(), "rbac.authorization.k8s.io/") {
		switch obj.GetKind() {
		case "ClusterRole", "ClusterRoleBinding":
			return true
		}
	}
	return strings.EqualFold(obj.GetKind(), "CustomResourceDefinition")
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
		r.Recorder = mgr.GetEventRecorderFor("application-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Application{}).
		Watches(&corev1alpha1.Repository{}, handler.EnqueueRequestsFromMapFunc(r.applicationsForRepository)).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		Watches(&corev1.Service{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		Watches(&appsv1.Deployment{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		Watches(&appsv1.StatefulSet{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		Watches(&appsv1.DaemonSet{}, handler.EnqueueRequestsFromMapFunc(managedObjectToApplication)).
		Named("application").
		Complete(r)
}
