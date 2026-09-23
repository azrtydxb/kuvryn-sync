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
	"strings"

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
	paths, err := solderConfigPaths(repository)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, configPath := range paths {
		apps, found, err := applicationsFromSolderFile(repository, resolved.CacheDir, configPath)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		for i := range apps {
			app, err := normalizeDiscoveredApplication(repository, configPath, i, apps[i], seen)
			if err != nil {
				return err
			}
			if err := r.upsertDiscoveredApplication(ctx, repository, &app, configPath); err != nil {
				return err
			}
			seen[app.Name] = struct{}{}
		}
	}
	return r.pruneRemovedDiscoveredApplications(ctx, repository, seen)
}

func normalizeDiscoveredApplication(repository *corev1alpha1.Repository, configPath string, index int, app corev1alpha1.Application, seen map[string]struct{}) (corev1alpha1.Application, error) {
	if app.Name == "" {
		return app, fmt.Errorf("%s application at index %d is missing metadata.name", configPath, index)
	}
	if _, duplicate := seen[app.Name]; duplicate {
		return app, fmt.Errorf("application %q is declared by more than one Solder config file", app.Name)
	}
	if app.Namespace == "" {
		app.Namespace = repository.Namespace
	}
	if app.Namespace != repository.Namespace {
		return app, fmt.Errorf("%s application %q must stay in repository namespace %q", configPath, app.Name, repository.Namespace)
	}
	if app.Spec.Source.RepositoryRef.Name == "" {
		app.Spec.Source.RepositoryRef.Name = repository.Name
	}
	if app.Spec.Source.RepositoryRef.Name != repository.Name {
		return app, fmt.Errorf("%s application %q references repository %q instead of %q", configPath, app.Name, app.Spec.Source.RepositoryRef.Name, repository.Name)
	}
	if app.Spec.Source.Render.Type == "" {
		return app, fmt.Errorf("%s application %q must set spec.source.render.type", configPath, app.Name)
	}
	// Git write access must not choose which service account Solder acts as;
	// the Repository owner decides.
	pinned := repository.Spec.ApplicationServiceAccountName
	if name := app.Spec.ServiceAccountName; name != "" && name != pinned {
		if pinned == "" {
			return app, fmt.Errorf("%s application %q may not set serviceAccountName; set spec.applicationServiceAccountName on Repository %q", configPath, app.Name, repository.Name)
		}
		return app, fmt.Errorf("%s application %q names service account %q but Repository %q pins %q", configPath, app.Name, name, repository.Name, pinned)
	}
	app.Spec.ServiceAccountName = pinned
	// Approvals come from people through the admission webhook, never from Git.
	for _, key := range []string{corev1alpha1.ApprovedRevisionAnnotation, corev1alpha1.ApprovedByAnnotation, corev1alpha1.ApprovedAtAnnotation, corev1alpha1.ApprovedDigestAnnotation} {
		delete(app.Annotations, key)
	}
	return app, nil
}

func solderConfigPaths(repository *corev1alpha1.Repository) ([]string, error) {
	paths := repository.Spec.ApplicationConfigPaths
	if len(paths) == 0 {
		paths = []string{solderConfigFileName}
	}
	out := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, raw := range paths {
		path := filepath.ToSlash(strings.TrimSpace(raw))
		if path == "" {
			return nil, fmt.Errorf("applicationConfigPaths must not contain empty paths")
		}
		if filepath.IsAbs(path) {
			return nil, fmt.Errorf("applicationConfigPaths path %q must be repository-relative", raw)
		}
		clean := filepath.ToSlash(filepath.Clean(path))
		if clean == "." || !filepath.IsLocal(clean) {
			return nil, fmt.Errorf("applicationConfigPaths path %q must stay inside the repository", raw)
		}
		if filepath.Base(clean) != solderConfigFileName {
			return nil, fmt.Errorf("applicationConfigPaths path %q must name a %s file", raw, solderConfigFileName)
		}
		if _, duplicate := seen[clean]; duplicate {
			return nil, fmt.Errorf("applicationConfigPaths path %q is listed more than once", clean)
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out, nil
}

func applicationsFromSolderFile(repository *corev1alpha1.Repository, workspace, configPath string) ([]corev1alpha1.Application, bool, error) {
	path := filepath.Join(workspace, filepath.FromSlash(configPath))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("could not read %s: %w", configPath, err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, true, fmt.Errorf("could not parse %s: %w", configPath, err)
	}
	if _, ok := raw["applications"]; ok {
		var envelope solderRepositoryFile
		if err := yaml.Unmarshal(data, &envelope); err != nil {
			return nil, true, fmt.Errorf("could not parse %s: %w", configPath, err)
		}
		return envelope.Applications, true, nil
	}
	var app corev1alpha1.Application
	if err := yaml.Unmarshal(data, &app); err != nil {
		return nil, true, fmt.Errorf("could not parse %s: %w", configPath, err)
	}
	if app.Kind == "Application" || app.Name != "" || app.Spec.Source.Render.Type != "" || app.Spec.Source.Path != "" {
		return []corev1alpha1.Application{app}, true, nil
	}
	return nil, true, fmt.Errorf("%s must contain kind: Application or an applications list for repository %q", configPath, repository.Name)
}

// upsertDiscoveredApplication creates or updates the Application a Solder
// config file declares, refusing one another controller owns.
func (r *RepositoryReconciler) upsertDiscoveredApplication(ctx context.Context, repository *corev1alpha1.Repository, desired *corev1alpha1.Application, configPath string) error {
	app := &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: desired.Namespace, Name: desired.Name}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, app, func() error {
		if app.ResourceVersion == "" {
			// A new Application takes all its metadata from the config file;
			// an existing one keeps its own.
			app.ObjectMeta = desired.ObjectMeta
		}
		app.Spec = desired.Spec
		ensureDiscoveredApplicationMetadata(repository, app, configPath)
		return controllerutil.SetControllerReference(repository, app, r.Scheme)
	})
	return err
}

func ensureDiscoveredApplicationMetadata(repository *corev1alpha1.Repository, app *corev1alpha1.Application, configPath string) {
	metav1.SetMetaDataLabel(&app.ObjectMeta, repositoryApplicationLabel, repository.Name)
	metav1.SetMetaDataAnnotation(&app.ObjectMeta, "solder.io/discovered-from", configPath)
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
