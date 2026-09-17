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
	"fmt"
	"os"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/source"
)

type solderRepositoryFile struct {
	metav1.TypeMeta `json:",inline"`
	Applications    []corev1alpha1.Application `json:"applications,omitempty"`
}

func (r *RepositoryReconciler) reconcileDiscoveredApplications(ctx context.Context, repository *corev1alpha1.Repository, resolved source.ResolvedSource) error {
	apps, found, err := applicationsFromSolderFile(repository, resolved.CacheDir)
	if err != nil {
		return err
	}
	if !found {
		return r.pruneRemovedDiscoveredApplications(ctx, repository, map[string]struct{}{})
	}
	seen := map[string]struct{}{}
	for i := range apps {
		app := apps[i]
		if app.Name == "" {
			return fmt.Errorf("%s application at index %d is missing metadata.name", solderConfigFileName, i)
		}
		if app.Namespace == "" {
			app.Namespace = repository.Namespace
		}
		if app.Namespace != repository.Namespace {
			return fmt.Errorf("%s application %q must stay in repository namespace %q", solderConfigFileName, app.Name, repository.Namespace)
		}
		if app.Spec.Source.RepositoryRef.Name == "" {
			app.Spec.Source.RepositoryRef.Name = repository.Name
		}
		if app.Spec.Source.RepositoryRef.Name != repository.Name {
			return fmt.Errorf("%s application %q references repository %q instead of %q", solderConfigFileName, app.Name, app.Spec.Source.RepositoryRef.Name, repository.Name)
		}
		if app.Spec.Source.Render.Type == "" {
			return fmt.Errorf("%s application %q must set spec.source.render.type", solderConfigFileName, app.Name)
		}
		if err := r.upsertDiscoveredApplication(ctx, repository, &app); err != nil {
			return err
		}
		seen[app.Name] = struct{}{}
	}
	return r.pruneRemovedDiscoveredApplications(ctx, repository, seen)
}

func applicationsFromSolderFile(repository *corev1alpha1.Repository, workspace string) ([]corev1alpha1.Application, bool, error) {
	path := filepath.Join(workspace, solderConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("could not read %s: %w", solderConfigFileName, err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, true, fmt.Errorf("could not parse %s: %w", solderConfigFileName, err)
	}
	if _, ok := raw["applications"]; ok {
		var envelope solderRepositoryFile
		if err := yaml.Unmarshal(data, &envelope); err != nil {
			return nil, true, fmt.Errorf("could not parse %s: %w", solderConfigFileName, err)
		}
		return envelope.Applications, true, nil
	}
	var app corev1alpha1.Application
	if err := yaml.Unmarshal(data, &app); err != nil {
		return nil, true, fmt.Errorf("could not parse %s: %w", solderConfigFileName, err)
	}
	if app.Kind == "Application" || app.Name != "" || app.Spec.Source.Render.Type != "" || app.Spec.Source.Path != "" {
		return []corev1alpha1.Application{app}, true, nil
	}
	return nil, true, fmt.Errorf("%s must contain kind: Application or an applications list for repository %q", solderConfigFileName, repository.Name)
}

func (r *RepositoryReconciler) upsertDiscoveredApplication(ctx context.Context, repository *corev1alpha1.Repository, desired *corev1alpha1.Application) error {
	key := client.ObjectKey{Namespace: desired.Namespace, Name: desired.Name}
	current := &corev1alpha1.Application{}
	if err := r.Get(ctx, key, current); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		desired.TypeMeta = metav1.TypeMeta{APIVersion: corev1alpha1.GroupVersion.String(), Kind: "Application"}
		ensureDiscoveredApplicationMetadata(repository, desired)
		if r.Scheme != nil {
			if err := controllerutil.SetControllerReference(repository, desired, r.Scheme); err != nil {
				return err
			}
		}
		return r.Create(ctx, desired)
	}
	current.Spec = desired.Spec
	ensureDiscoveredApplicationMetadata(repository, current)
	if r.Scheme != nil {
		if err := controllerutil.SetControllerReference(repository, current, r.Scheme); err != nil {
			return err
		}
	}
	return r.Update(ctx, current)
}

func ensureDiscoveredApplicationMetadata(repository *corev1alpha1.Repository, app *corev1alpha1.Application) {
	labels := app.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[repositoryApplicationLabel] = repository.Name
	app.SetLabels(labels)
	annotations := app.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["solder.io/discovered-from"] = solderConfigFileName
	app.SetAnnotations(annotations)
}

func (r *RepositoryReconciler) pruneRemovedDiscoveredApplications(ctx context.Context, repository *corev1alpha1.Repository, seen map[string]struct{}) error {
	var list corev1alpha1.ApplicationList
	if err := r.List(ctx, &list, client.InNamespace(repository.Namespace), client.MatchingLabels{repositoryApplicationLabel: repository.Name}); err != nil {
		return err
	}
	for i := range list.Items {
		app := &list.Items[i]
		if _, ok := seen[app.Name]; ok {
			continue
		}
		if err := r.Delete(ctx, app); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}
