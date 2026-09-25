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
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/imageupdate"
	"github.com/azrtydxb/kuvryn-sync/internal/source"
	gitcache "github.com/azrtydxb/kuvryn-sync/internal/source/git"
)

const (
	defaultSourceCacheDir      = "solder-source-cache"
	solderConfigFileName       = ".solder.yaml"
	repositoryApplicationLabel = "solder.io/repository"
)

// RepositoryReconciler reconciles a Repository object.
type RepositoryReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	SourceResolver source.Resolver
	CacheDir       string
	// ImageUpdater commits ImagePolicy selections back to Git; nil disables it.
	ImageUpdater *imageupdate.Updater
}

// +kubebuilder:rbac:groups=solder.io,resources=repositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solder.io,resources=repositories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solder.io,resources=repositories/finalizers,verbs=update
// +kubebuilder:rbac:groups=solder.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile resolves Git repositories to immutable source revisions and records
// source readiness without exposing credentials.
func (r *RepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	repository := &corev1alpha1.Repository{}
	if err := r.Get(ctx, req.NamespacedName, repository); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	result := ctrl.Result{}
	if repository.Spec.PollInterval != nil {
		result.RequeueAfter = repository.Spec.PollInterval.Duration
	}

	if repository.Spec.Type != "" && repository.Spec.Type != corev1alpha1.RepositoryTypeGit {
		r.markFailed(repository, source.FailureReasonValidationFailure, "Only Git repositories are supported")
		return result, r.updateStatus(ctx, repository)
	}
	if repository.Spec.Git == nil {
		r.markFailed(repository, source.FailureReasonValidationFailure, "Git repository configuration is required")
		return result, r.updateStatus(ctx, repository)
	}

	credentials, err := r.loadGitCredentials(ctx, repository)
	if err != nil {
		r.markSourceError(repository, err)
		return result, r.updateStatus(ctx, repository)
	}

	resolved, err := r.resolver().Resolve(ctx, source.GitRepository{
		URL:      repository.Spec.Git.URL,
		Revision: repository.Spec.Git.Revision,
		Auth:     credentials,
	})
	if err != nil {
		r.markSourceError(repository, err)
		return result, r.updateStatus(ctx, repository)
	}

	if err := r.reconcileDiscoveredApplications(ctx, repository, resolved); err != nil {
		r.markFailed(repository, source.FailureReasonValidationFailure, safeMessage(err, "Repository application discovery failed"))
		return result, r.updateStatus(ctx, repository)
	}

	if pushed := r.updateImages(ctx, repository); pushed {
		// Fetch the commit just pushed instead of waiting for the next poll.
		result.RequeueAfter = time.Second
	}

	now := metav1.Now()
	repository.Status.State = corev1alpha1.RepositoryStateReady
	repository.Status.ObservedRevision = resolved.Revision
	repository.Status.LastFetchedAt = &now
	apimeta.SetStatusCondition(&repository.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "FetchSucceeded",
		Message:            "Git source resolved successfully",
		ObservedGeneration: repository.Generation,
	})
	r.event(repository, corev1.EventTypeNormal, "RepositoryReady", "Git source resolved successfully")
	log.Info("Resolved Git repository", "repository", repository.Name, "namespace", repository.Namespace, "revision", resolved.Revision)

	return result, r.updateStatus(ctx, repository)
}

// cacheRoot returns dir, or the default source cache under the system
// temporary directory when dir is empty.
func cacheRoot(dir string) string {
	return cmp.Or(dir, filepath.Join(os.TempDir(), defaultSourceCacheDir))
}

// chartCacheSubdir holds pulled Helm charts under the source cache root.
const chartCacheSubdir = "charts"

// chartCacheDir is where pulled Helm charts are cached, beside the Git cache.
func chartCacheDir(dir string) string {
	return filepath.Join(cacheRoot(dir), chartCacheSubdir)
}

func (r *RepositoryReconciler) resolver() source.Resolver {
	if r.SourceResolver != nil {
		return r.SourceResolver
	}
	r.SourceResolver = gitcache.NewCache(cacheRoot(r.CacheDir))
	return r.SourceResolver
}

// GitCredentialsLabel marks a Secret that Solder may use as Git credentials.
const GitCredentialsLabel = "solder.io/git-credentials"

func (r *RepositoryReconciler) loadGitCredentials(ctx context.Context, repository *corev1alpha1.Repository) (source.Credentials, error) {
	if repository.Spec.Git == nil || repository.Spec.Git.Auth == nil || repository.Spec.Git.Auth.SecretRef == nil {
		return source.Credentials{}, nil
	}
	return r.credentialsFromSecret(ctx, repository.Namespace, repository.Spec.Git.Auth.SecretRef.Name)
}

// credentialsFromSecret loads Git credentials from a labelled Secret.
func (r *RepositoryReconciler) credentialsFromSecret(ctx context.Context, namespace, name string) (source.Credentials, error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := r.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return source.Credentials{}, &source.Error{Reason: source.FailureReasonAuthenticationFailure, Message: "Git authentication Secret was not found", Err: err}
		}
		return source.Credentials{}, err
	}
	// Whoever writes a Repository chooses both the Git URL and the Secret, so
	// only Secrets explicitly marked as Git credentials may be sent anywhere.
	if secret.Labels[GitCredentialsLabel] != "true" {
		return source.Credentials{}, &source.Error{Reason: source.FailureReasonAuthenticationFailure, Message: fmt.Sprintf("Git authentication Secret is not labelled %s=true", GitCredentialsLabel)}
	}
	credentials := source.Credentials{
		Username:   string(secret.Data["username"]),
		Password:   firstSecretString(secret, "password", "basic-password"),
		Token:      firstSecretString(secret, "token", "accessToken", "access_token"),
		SSHKey:     firstSecretString(secret, "sshPrivateKey", "ssh-privatekey", "identity"),
		KnownHosts: firstSecretString(secret, "known_hosts", "knownHosts"),
	}
	if credentials.Username == "" && credentials.Password == "" && credentials.Token == "" && credentials.SSHKey == "" && credentials.KnownHosts == "" {
		return source.Credentials{}, &source.Error{Reason: source.FailureReasonAuthenticationFailure, Message: "Git authentication Secret contains no supported credential keys"}
	}
	return credentials, nil
}

func firstSecretString(secret *corev1.Secret, keys ...string) string {
	for _, key := range keys {
		if value := string(secret.Data[key]); value != "" {
			return value
		}
	}
	return ""
}

func (r *RepositoryReconciler) markSourceError(repository *corev1alpha1.Repository, err error) {
	var sourceErr *source.Error
	if errors.As(err, &sourceErr) {
		r.markFailed(repository, sourceErr.Reason, sourceErr.Message)
		return
	}
	r.markFailed(repository, source.FailureReasonSourceFailure, "Repository source reconciliation failed")
}

func (r *RepositoryReconciler) markFailed(repository *corev1alpha1.Repository, reason source.FailureReason, message string) {
	repository.Status.State = corev1alpha1.RepositoryStateFailed
	apimeta.SetStatusCondition(&repository.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: repository.Generation,
	})
	r.event(repository, corev1.EventTypeWarning, string(reason), message)
}

func (r *RepositoryReconciler) updateStatus(ctx context.Context, repository *corev1alpha1.Repository) error {
	return r.Status().Update(ctx, repository)
}

func (r *RepositoryReconciler) event(repository *corev1alpha1.Repository, eventType, reason, message string) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Event(repository, eventType, reason, message)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		// The deprecated core recorder stays: the events.k8s.io recorder merges
		// events that differ only in message; see .procoder/todo.
		r.Recorder = mgr.GetEventRecorderFor("repository-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Repository{}).
		Watches(&corev1alpha1.ImagePolicy{}, handler.EnqueueRequestsFromMapFunc(r.repositoriesForImagePolicy), builder.WithPredicates(latestImageChanged)).
		Named("repository").
		Complete(r)
}

// latestImageChanged admits ImagePolicy updates that select another image.
// Every scan writes the policy's status, and each of those writes would
// otherwise cost a Git fetch per Repository in the namespace.
var latestImageChanged = predicate.Funcs{UpdateFunc: func(e event.UpdateEvent) bool {
	old, oldOK := e.ObjectOld.(*corev1alpha1.ImagePolicy)
	updated, updatedOK := e.ObjectNew.(*corev1alpha1.ImagePolicy)
	return !oldOK || !updatedOK || old.Status.LatestImage != updated.Status.LatestImage
}}

// updateImages commits the images selected by ImagePolicies in the
// Repository's namespace wherever the repository has markers for them, and
// records the outcome in the ImagesUpdated condition. It never makes the
// Repository NotReady and reports whether a commit was pushed.
func (r *RepositoryReconciler) updateImages(ctx context.Context, repository *corev1alpha1.Repository) bool {
	spec := repository.Spec.ImageUpdate
	if spec == nil {
		apimeta.RemoveStatusCondition(&repository.Status.Conditions, "ImagesUpdated")
		return false
	}
	if r.ImageUpdater == nil {
		apimeta.SetStatusCondition(&repository.Status.Conditions, metav1.Condition{Type: "ImagesUpdated", Status: metav1.ConditionFalse, Reason: "Disabled", Message: "Image write-back is not enabled in this controller", ObservedGeneration: repository.Generation})
		return false
	}
	condition := metav1.Condition{Type: "ImagesUpdated", Status: metav1.ConditionTrue, Reason: "UpToDate", Message: "Image references match their ImagePolicies", ObservedGeneration: repository.Generation}
	commit, changes, err := r.writeBackImages(ctx, repository, spec)
	switch {
	case err != nil:
		condition.Status, condition.Reason, condition.Message = metav1.ConditionFalse, "UpdateFailed", safeMessage(err, "Image update failed")
		r.event(repository, corev1.EventTypeWarning, "ImageUpdateFailed", condition.Message)
	case commit != "":
		condition.Reason = "Committed"
		condition.Message = fmt.Sprintf("Pushed %s updating %d image reference(s)", commit, len(changes))
		r.event(repository, corev1.EventTypeNormal, "ImagesUpdated", condition.Message)
	}
	apimeta.SetStatusCondition(&repository.Status.Conditions, condition)
	return commit != ""
}

func (r *RepositoryReconciler) writeBackImages(ctx context.Context, repository *corev1alpha1.Repository, spec *corev1alpha1.ImageUpdateSpec) (string, []imageupdate.Change, error) {
	branch := spec.Branch
	if branch == "" {
		branch = repository.Spec.Git.Revision
	}
	if branch == "" {
		return "", nil, fmt.Errorf("set spec.imageUpdate.branch or spec.git.revision")
	}
	credentials, err := r.credentialsFromSecret(ctx, repository.Namespace, spec.SecretRef.Name)
	if err != nil {
		return "", nil, err
	}
	var policies corev1alpha1.ImagePolicyList
	if err := r.List(ctx, &policies, client.InNamespace(repository.Namespace)); err != nil {
		return "", nil, err
	}
	images := map[string]imageupdate.Image{}
	for _, policy := range policies.Items {
		if policy.Status.LatestImage != "" {
			images[policy.Namespace+":"+policy.Name] = imageupdate.Image{Name: policy.Spec.Image, Tag: policy.Status.LatestTag, Full: policy.Status.LatestImage}
		}
	}
	if len(images) == 0 {
		return "", nil, nil
	}
	return r.ImageUpdater.Update(ctx, imageupdate.Request{
		URL: repository.Spec.Git.URL, Branch: branch, Path: spec.Path, Auth: credentials,
		Namespace: repository.Namespace, Images: images,
		Author: object.Signature{Name: spec.AuthorName, Email: spec.AuthorEmail},
	})
}

// repositoriesForImagePolicy re-queues Repositories that write back images
// in the ImagePolicy's namespace.
func (r *RepositoryReconciler) repositoriesForImagePolicy(ctx context.Context, obj client.Object) []reconcile.Request {
	var repositories corev1alpha1.RepositoryList
	if err := r.List(ctx, &repositories, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	requests := []reconcile.Request{}
	for _, repository := range repositories.Items {
		if repository.Spec.ImageUpdate != nil {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&repository)})
		}
	}
	return requests
}
