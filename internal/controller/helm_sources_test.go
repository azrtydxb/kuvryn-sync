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
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	chartv2loader "helm.sh/helm/v4/pkg/chart/v2/loader"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	repo "helm.sh/helm/v4/pkg/repo/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/renderer"
	helmrenderer "github.com/azrtydxb/kuvryn-sync/internal/renderer/helm"
)

var _ = Describe("Helm charts from a repository with valuesFrom", func() {
	const appName = "chart-app"
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}
	var server *httptest.Server
	var archiveDigest, caFile string

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		src := GinkgoT().TempDir()
		files := map[string]string{
			"Chart.yaml":               "apiVersion: v2\nname: app\nversion: 0.1.0\n",
			"values.yaml":              "level: chart-default\nregion: none\n",
			"templates/configmap.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app-config\n  annotations:\n    level: {{ .Values.level | quote }}\n    region: {{ .Values.region | quote }}\n",
		}
		for name, content := range files {
			Expect(os.MkdirAll(filepath.Join(src, "app", filepath.Dir(name)), 0o700)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(src, "app", name), []byte(content), 0o600)).To(Succeed())
		}
		loaded, err := chartv2loader.LoadDir(filepath.Join(src, "app"))
		Expect(err).NotTo(HaveOccurred())
		repoDir := GinkgoT().TempDir()
		archive, err := chartutil.Save(loaded, repoDir)
		Expect(err).NotTo(HaveOccurred())
		data, err := os.ReadFile(archive)
		Expect(err).NotTo(HaveOccurred())
		sum := sha256.Sum256(data)
		archiveDigest = "sha256:" + hex.EncodeToString(sum[:])
		server = httptest.NewTLSServer(http.FileServer(http.Dir(repoDir)))
		caFile = filepath.Join(GinkgoT().TempDir(), "ca.pem")
		Expect(os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600)).To(Succeed())
		index, err := repo.IndexDirectory(repoDir, server.URL)
		Expect(err).NotTo(HaveOccurred())
		Expect(index.WriteFile(filepath.Join(repoDir, "index.yaml"), 0o600)).To(Succeed())

		createRepository(ctx)
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "chart-values", Namespace: "default"}, Data: map[string]string{"values.yaml": "level: from-configmap\n"}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "chart-secret-values", Namespace: "default"}, Data: map[string][]byte{"values.yaml": []byte("level: s3cr3t-level\n")}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments", Annotations: map[string]string{"level": "live", "region": "live"}}})).To(Succeed())
	})

	AfterEach(func() {
		server.Close()
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "chart-values", Namespace: "default"}})
		deleteObject(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "chart-secret-values", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		deleteApplicationRevisions(ctx, appName)
	})

	It("pulls the pinned chart, records its digest, merges values in order, and hides Secret values", func() {
		app := newApplication(appName, corev1alpha1.RenderTypeHelm)
		// The pre-created ConfigMap is owned by the test client; adopt it.
		app.Spec.Sync.ConflictPolicy = corev1alpha1.ConflictPolicyAdopt
		app.Spec.Source.Render.Helm = &corev1alpha1.HelmRenderSpec{
			ReleaseName: "payments",
			Chart:       &corev1alpha1.HelmChartSource{Repository: server.URL, Name: "app", Version: "0.1.0"},
			ValuesFrom: []corev1alpha1.HelmValuesReference{
				{Kind: "ConfigMap", Name: "chart-values"},
				{Kind: "Secret", Name: "chart-secret-values"},
			},
			Values: &apiextensionsv1.JSON{Raw: []byte(`{"region":"eu-west"}`)},
		}
		Expect(k8sClient.Create(ctx, app)).To(Succeed())

		reconciler := newApplicationReconciler(nil, nil)
		reconciler.Renderers = func(corev1alpha1.RenderType) (renderer.Renderer, error) { return helmrenderer.Renderer{}, nil }
		reconciler.SourceResolver = workspaceResolver(GinkgoT().TempDir())
		reconciler.CacheDir = GinkgoT().TempDir()
		reconciler.ChartCAFile = caFile
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Failure).To(BeNil())
		Expect(revision.Status.ChartDigest).To(Equal(archiveDigest))
		status, err := json.Marshal(revision.Status)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(status)).NotTo(ContainSubstring("s3cr3t-level"))
		Expect(string(status)).To(ContainSubstring("eu-west"), "inline values are merged last")

		changes := map[string]corev1alpha1.PlanFieldChange{}
		for _, change := range revision.Status.Plan.Resources[0].Changes {
			changes[change.Path] = change
		}
		Expect(changes["metadata.annotations.level"].Redacted).To(BeTrue(), "the Secret's value won over the ConfigMap's and must be masked")
		Expect(changes["metadata.annotations.region"].After).To(Equal("eu-west"))
	})
})
