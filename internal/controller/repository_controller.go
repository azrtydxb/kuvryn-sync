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
	"errors"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/source"
	gitcache "github.com/azrtydxb/solder/internal/source/git"
)

const defaultSourceCacheDir = "solder-source-cache"

// RepositoryReconciler reconciles a Repository object.
type RepositoryReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	SourceResolver source.Resolver
	CacheDir       string
}

// +kubebuilder:rbac:groups=solder.io,resources=repositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solder.io,resources=repositories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solder.io,resources=repositories/finalizers,verbs=update
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

func (r *RepositoryReconciler) resolver() source.Resolver {
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

func (r *RepositoryReconciler) loadGitCredentials(ctx context.Context, repository *corev1alpha1.Repository) (source.Credentials, error) {
	if repository.Spec.Git == nil || repository.Spec.Git.Auth == nil || repository.Spec.Git.Auth.SecretRef == nil {
		return source.Credentials{}, nil
	}
	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: repository.Namespace, Name: repository.Spec.Git.Auth.SecretRef.Name}
	if err := r.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return source.Credentials{}, &source.Error{Reason: source.FailureReasonAuthenticationFailure, Message: "Git authentication Secret was not found", Err: err}
		}
		return source.Credentials{}, err
	}
	credentials := source.Credentials{
		Username:   secretString(secret, "username"),
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

func secretString(secret *corev1.Secret, key string) string {
	value, ok := secret.Data[key]
	if !ok {
		return ""
	}
	return string(value)
}

func firstSecretString(secret *corev1.Secret, keys ...string) string {
	for _, key := range keys {
		if value := secretString(secret, key); value != "" {
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
		r.Recorder = mgr.GetEventRecorderFor("repository-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Repository{}).
		Named("repository").
		Complete(r)
}
