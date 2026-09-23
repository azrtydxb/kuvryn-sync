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
	"slices"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

func TestKeptCommitsCoverRevisionsAndObservedCommits(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	revision := func(namespace, name, repository, commit string) *corev1alpha1.Revision {
		return &corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec: corev1alpha1.RevisionSpec{Source: corev1alpha1.RevisionSource{
				RepositoryRef: corev1alpha1.LocalObjectReference{Name: repository}, Revision: commit,
			}},
		}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "platform"},
			Spec:       corev1alpha1.RepositorySpec{Git: &corev1alpha1.GitRepositorySpec{URL: "https://git.example/platform.git"}},
			Status:     corev1alpha1.RepositoryStatus{ObservedRevision: "c3"},
		},
		&corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "no-git"}},
		revision("team-a", "platform-c1", "platform", "c1"),
		revision("team-a", "platform-c2", "platform", "c2"),
		// Same Repository name in another namespace is a different Repository.
		revision("team-b", "platform-c9", "platform", "c9"),
	).Build()

	keep, err := keptCommits(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	commits := keep["https://git.example/platform.git"]
	slices.Sort(commits)
	if !slices.Equal(commits, []string{"c1", "c2", "c3"}) || len(keep) != 1 {
		t.Fatalf("keep = %v, want platform.git with c1, c2, c3 only", keep)
	}
}
