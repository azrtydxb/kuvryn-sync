//go:build e2e
// +build e2e

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

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/azrtydxb/solder/test/utils"
)

// namespace where the project is deployed in
const namespace = "solder-system"

// serviceAccountName created for the project
const serviceAccountName = "solder-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "solder-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "solder-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace, "--dry-run=client", "-o", "yaml")
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to render namespace")
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = bytes.NewBufferString(out)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				By("getting the name of the controller-manager pod")
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				By("validating the pod's status")
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=solder-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
				"--dry-run=client", "-o", "yaml",
			)
			out, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to render ClusterRoleBinding")
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewBufferString(out)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("creating the curl-metrics pod to access the metrics endpoint")
			_, _ = utils.Run(exec.Command(
				"kubectl", "delete", "pod", "curl-metrics", "-n", namespace, "--ignore-not-found=true",
			))
			overrides, err := metricsPodOverride(token)
			Expect(err).NotTo(HaveOccurred())
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides", overrides)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		It("should reconcile a GitHub-backed Solder Application end to end", func() {
			manifestPath := writeTempManifest(productApplicationManifest)

			By("applying a Repository and Application that render plain YAML from Git")
			cmd := exec.Command("kubectl", "apply", "-f", manifestPath)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply product e2e resources")

			DeferCleanup(func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", manifestPath, "--ignore-not-found=true"))
				_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", "solder-e2e-product", "--ignore-not-found=true"))
			})

			By("waiting for the Repository to resolve an immutable Git revision")
			Eventually(func(g Gomega) {
				cmd := exec.Command(
					"kubectl", "get", "repository", "solder-e2e-product-repo", "-o",
					"jsonpath={.status.state}:{.status.observedRevision}",
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(MatchRegexp(`^Ready:[0-9a-f]{40}$`))
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for the Application to become synced and healthy")
			Eventually(func(g Gomega) {
				cmd := exec.Command(
					"kubectl", "get", "application", "solder-e2e-product", "-o",
					"jsonpath={.status.sync.state}:{.status.health.state}:{.status.desiredRevision}:{.status.deployedRevision}",
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				parts := utils.GetNonEmptyLines(output)
				g.Expect(parts).To(HaveLen(1))
				g.Expect(parts[0]).To(MatchRegexp(`^Synced:Healthy:[0-9a-f]{40}:[0-9a-f]{40}$`))
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying the rendered Kubernetes object was applied")
			Eventually(func(g Gomega) {
				cmd := exec.Command(
					"kubectl", "get", "configmap", "solder-e2e-config", "-n", "solder-e2e-product", "-o",
					"jsonpath={.data.source}:{.data.version}:{.metadata.annotations.solder\\.io/revision}",
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(MatchRegexp(`^github:v[0-9]+:solder-e2e-product-[0-9a-f]+$`))
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying a healthy Revision was recorded")
			cmd = exec.Command(
				"kubectl", "get", "revision", "-l", "solder.io/application=solder-e2e-product", "-o",
				"jsonpath={.items[0].status.phase}",
			)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Healthy"))
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	By("creating temporary file to store the token request")
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		By("executing kubectl command to create the token")
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		By("parsing the JSON output to extract the token")
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

func metricsPodOverride(token string) (string, error) {
	curlCommand := fmt.Sprintf(
		"for i in $(seq 1 30); do curl -v -k -H 'Authorization: Bearer %s' "+
			"https://%s.%s.svc.cluster.local:8443/metrics && exit 0 || sleep 2; done; exit 1",
		token,
		metricsServiceName,
		namespace,
	)
	overrides := map[string]any{
		"spec": map[string]any{
			"serviceAccountName": serviceAccountName,
			"containers": []map[string]any{{
				"name":    "curl",
				"image":   "curlimages/curl:latest",
				"command": []string{"/bin/sh", "-c"},
				"args":    []string{curlCommand},
				"securityContext": map[string]any{
					"readOnlyRootFilesystem":   true,
					"allowPrivilegeEscalation": false,
					"capabilities":             map[string][]string{"drop": {"ALL"}},
					"runAsNonRoot":             true,
					"runAsUser":                1000,
					"seccompProfile":           map[string]string{"type": "RuntimeDefault"},
				},
			}},
		},
	}
	out, err := json.Marshal(overrides)
	return string(out), err
}

func writeTempManifest(content string) string {
	file, err := os.CreateTemp("", "solder-product-e2e-*.yaml")
	Expect(err).NotTo(HaveOccurred())
	_, err = file.WriteString(content)
	Expect(err).NotTo(HaveOccurred())
	Expect(file.Close()).To(Succeed())
	DeferCleanup(func() { _ = os.Remove(file.Name()) })
	return file.Name()
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}

const productApplicationManifest = `apiVersion: v1
kind: Namespace
metadata:
  name: solder-e2e-product
---
apiVersion: solder.io/v1alpha1
kind: Repository
metadata:
  name: solder-e2e-product-repo
spec:
  type: git
  git:
    url: https://github.com/azrtydxb/solder-e2e-app.git
    revision: main
  pollInterval: 30s
---
apiVersion: solder.io/v1alpha1
kind: Application
metadata:
  name: solder-e2e-product
spec:
  source:
    repositoryRef:
      name: solder-e2e-product-repo
    path: manifests
    render:
      type: yaml
  destination:
    namespace: solder-e2e-product
  sync:
    automatic: true
    prune: true
    selfHeal: true
    conflictPolicy: fail
  strategy:
    type: rolling
    failurePolicy:
      action: rollback
      timeout: 2m
  health:
    timeout: 2m
  history:
    limit: 5
`
