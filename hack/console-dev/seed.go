package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// sampleApp is one of the design's sample Applications.
type sampleApp struct {
	name, ns, repo, path, render, commit string
	sync                                 corev1alpha1.SyncState
	health                               corev1alpha1.HealthState
	last                                 time.Duration
	sa                                   string
	auto, noHeal                         bool
}

var apps = []sampleApp{
	{"payments", "payments", "platform", "apps/payments", "kustomize", "4e1c9a2", corev1alpha1.SyncStateOutOfSync, corev1alpha1.HealthStateDegraded, 4 * time.Minute, "payments-deployer", true, false},
	{"checkout", "shop", "platform", "apps/checkout", "kustomize", "9f2c1ab", corev1alpha1.SyncStateAwaitingApproval, corev1alpha1.HealthStateHealthy, 18 * time.Minute, "shop-deployer", false, false},
	{"catalog", "shop", "platform", "apps/catalog", "helm", "9f2c1ab", corev1alpha1.SyncStateSynced, corev1alpha1.HealthStateHealthy, 18 * time.Minute, "shop-deployer", true, false},
	{"search", "shop", "platform", "apps/search", "helm", "9f2c1ab", corev1alpha1.SyncStateDrifted, corev1alpha1.HealthStateHealthy, 2 * time.Minute, "shop-deployer", true, true},
	{"notifications", "shop", "platform", "apps/notifications", "yaml", "4e1c9a2", corev1alpha1.SyncStateApplying, corev1alpha1.HealthStateProgressing, 20 * time.Second, "shop-deployer", true, false},
	{"ingress-nginx", "ingress", "infra", "charts/ingress-nginx", "helm", "b71e04d", corev1alpha1.SyncStateSynced, corev1alpha1.HealthStateHealthy, 41 * time.Minute, "infra-deployer", true, false},
	{"cert-manager", "cert-manager", "infra", "charts/cert-manager", "helm", "b71e04d", corev1alpha1.SyncStateSynced, corev1alpha1.HealthStateHealthy, 41 * time.Minute, "infra-deployer", true, false},
	{"ledger", "finance", "finance-gitops", "ledger", "kustomize", "c0a8d33", corev1alpha1.SyncStateUnknown, corev1alpha1.HealthStateSuspended, 72 * time.Hour, "finance-deployer", true, false},
}

var shas = []string{"4e1c9a2", "9f2c1ab", "2d77b10", "81ce5f3", "b71e04d"}

// fullSHA pads a short commit to a 40-character one, as status records.
func fullSHA(short string) string {
	return short + strings.Repeat("0", 40-len(short))
}

func ago(d time.Duration) *metav1.Time {
	t := metav1.NewTime(time.Now().Add(-d).Truncate(time.Second))
	return &t
}

func ref(apiVersion, kind, ns, name string) corev1alpha1.ResourceRef {
	return corev1alpha1.ResourceRef{APIVersion: apiVersion, Kind: kind, Namespace: ns, Name: name}
}

// seed creates the viewer's RBAC, the design's sample Applications with
// their Revisions and managed objects, 3 Repositories and 3 ImagePolicies.
func seed(ctx context.Context, c client.Client) error {
	if err := seedRBAC(ctx, c); err != nil {
		return err
	}
	for _, ns := range []string{"payments", "shop", "ingress", "cert-manager", "finance"} {
		if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}); err != nil {
			return err
		}
	}
	if err := seedRepositories(ctx, c); err != nil {
		return err
	}
	for _, a := range apps {
		if err := seedApp(ctx, c, a); err != nil {
			return fmt.Errorf("application %s: %w", a.name, err)
		}
	}
	return seedImagePolicies(ctx, c)
}

func seedRBAC(ctx context.Context, c client.Client) error {
	read := []string{"get", "list", "watch"}
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "kuvryn-sync-viewer"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"sync.kuvryn.io"}, Resources: []string{"*"}, Verbs: read},
			{APIGroups: []string{""}, Resources: []string{"namespaces", "configmaps", "services", "serviceaccounts", "pods"}, Verbs: read},
			{APIGroups: []string{"apps"}, Resources: []string{"deployments", "replicasets", "statefulsets", "daemonsets"}, Verbs: read},
			{APIGroups: []string{"policy"}, Resources: []string{"poddisruptionbudgets"}, Verbs: read},
		},
	}
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "kuvryn-sync-viewers"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role.Name},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "Group", Name: "viewers"}},
	}
	for _, obj := range []client.Object{role, binding} {
		if err := c.Create(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

func seedRepositories(ctx context.Context, c client.Client) error {
	type sample struct {
		name, url, ref, commit string
		state                  corev1alpha1.RepositoryState
		poll                   time.Duration
		webhook                bool
		last                   time.Duration
		message                string
	}
	for _, r := range []sample{
		{"platform", "https://github.com/acme/platform.git", "main", "4e1c9a2", corev1alpha1.RepositoryStateReady, time.Minute, true, 4 * time.Minute, "Resolved main to 4e1c9a2"},
		{"infra", "https://github.com/acme/infra.git", "v2.14.0", "b71e04d", corev1alpha1.RepositoryStateReady, 5 * time.Minute, false, 41 * time.Minute, "Resolved v2.14.0 to b71e04d"},
		{"finance-gitops", "git@gitlab.acme.io:finance/gitops.git", "main", "c0a8d33", corev1alpha1.RepositoryStateFailed, time.Minute, true, 72 * time.Hour, "authentication required: Secret finance-deploy-key"},
	} {
		repo := &corev1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Namespace: appNamespace, Name: r.name},
			Spec: corev1alpha1.RepositorySpec{
				Type:         corev1alpha1.RepositoryTypeGit,
				Git:          &corev1alpha1.GitRepositorySpec{URL: r.url, Revision: r.ref, Auth: &corev1alpha1.GitAuthSpec{SecretRef: &corev1alpha1.SecretReference{Name: r.name + "-deploy-key"}}},
				PollInterval: &metav1.Duration{Duration: r.poll},
			},
		}
		if r.webhook {
			repo.Spec.Webhook = &corev1alpha1.RepositoryWebhook{SecretRef: corev1alpha1.SecretReference{Name: r.name + "-webhook"}}
		}
		if err := c.Create(ctx, repo); err != nil {
			return err
		}
		ready := metav1.ConditionTrue
		reason := "Resolved"
		if r.state != corev1alpha1.RepositoryStateReady {
			ready, reason = metav1.ConditionFalse, "SourceFailure"
		}
		repo.Status = corev1alpha1.RepositoryStatus{
			State:            r.state,
			ObservedRevision: fullSHA(r.commit),
			LastFetchedAt:    ago(r.last),
			Conditions:       []metav1.Condition{{Type: "Ready", Status: ready, Reason: reason, Message: r.message, LastTransitionTime: *ago(r.last)}},
		}
		if err := c.Status().Update(ctx, repo); err != nil {
			return err
		}
	}
	return nil
}

func seedImagePolicies(ctx context.Context, c client.Client) error {
	for _, p := range []struct{ name, image, rng, latest, digest string }{
		{"payments-api", "ghcr.io/acme/payments", ">=1.0.0", "1.9.0", "sha256:8c1f5b0e2a7d4c19e2a0"},
		{"checkout", "ghcr.io/acme/checkout", "~2.4", "2.4.7", "sha256:1d9347aa0c3e4b7c"},
		{"search", "ghcr.io/acme/search", "^0.12", "0.12.3", "sha256:77ae02b1d5c019"},
	} {
		policy := &corev1alpha1.ImagePolicy{
			ObjectMeta: metav1.ObjectMeta{Namespace: appNamespace, Name: p.name},
			Spec: corev1alpha1.ImagePolicySpec{
				Image:  p.image,
				Policy: corev1alpha1.ImageSelectionPolicy{Semver: &corev1alpha1.SemverPolicy{Range: p.rng}},
			},
		}
		if err := c.Create(ctx, policy); err != nil {
			return err
		}
		policy.Status = corev1alpha1.ImagePolicyStatus{LatestTag: p.latest, LatestDigest: p.digest, LatestImage: p.image + ":" + p.latest, LastScannedAt: ago(6 * time.Minute)}
		if err := c.Status().Update(ctx, policy); err != nil {
			return err
		}
	}
	return nil
}

func seedApp(ctx context.Context, c client.Client, a sampleApp) error {
	app := &corev1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Namespace: appNamespace, Name: a.name},
		Spec: corev1alpha1.ApplicationSpec{
			Source: corev1alpha1.ApplicationSource{
				RepositoryRef: corev1alpha1.LocalObjectReference{Name: a.repo},
				Revision:      map[bool]string{true: "v2.14.0", false: "main"}[a.repo == "infra"],
				Path:          a.path,
				Render:        corev1alpha1.RenderSpec{Type: corev1alpha1.RenderType(a.render)},
			},
			Destination:        corev1alpha1.ApplicationDestination{Namespace: a.ns},
			Sync:               corev1alpha1.SyncPolicy{Automatic: a.auto, Prune: true, SelfHeal: !a.noHeal, ConflictPolicy: corev1alpha1.ConflictPolicyFail},
			Strategy:           corev1alpha1.DeploymentStrategy{FailurePolicy: corev1alpha1.FailurePolicy{Action: corev1alpha1.FailureActionRollback, Timeout: &metav1.Duration{Duration: 5 * time.Minute}}},
			Suspend:            a.health == corev1alpha1.HealthStateSuspended,
			ServiceAccountName: a.sa,
		},
	}
	if err := c.Create(ctx, app); err != nil {
		return err
	}
	kinds := []corev1alpha1.ManagedKind{
		{APIVersion: "apps/v1", Kind: "Deployment"}, {APIVersion: "v1", Kind: "Service"}, {APIVersion: "v1", Kind: "ConfigMap"},
		{APIVersion: "v1", Kind: "ServiceAccount"}, {APIVersion: "policy/v1", Kind: "PodDisruptionBudget"},
	}
	if a.name == "payments" {
		kinds = append(kinds, corev1alpha1.ManagedKind{APIVersion: "v1", Kind: "Secret"})
	}
	app.Status = corev1alpha1.ApplicationStatus{
		State:              a.health,
		DesiredRevision:    fullSHA(a.commit),
		DeployedRevision:   fullSHA(a.commit),
		ServiceAccountName: a.sa,
		Sync:               corev1alpha1.ApplicationSyncStatus{State: a.sync},
		Health:             corev1alpha1.ApplicationHealthStatus{State: a.health},
		ManagedKinds:       kinds,
		Conditions:         conditions(a),
	}
	if a.name == "payments" {
		app.Status.Diagnosis = []corev1alpha1.DiagnosisCause{{
			Resource: ref("v1", "Secret", "payments", "db"),
			Reason:   "MissingSecret",
			Message:  `Secret payments/db does not exist; Pod api-7d9f-x2k: CreateContainerConfigError: secret "db" not found.`,
			Chain: []corev1alpha1.ResourceRef{
				ref("apps/v1", "Deployment", "payments", "api"),
				ref("apps/v1", "ReplicaSet", "payments", "api-7d9f"),
				ref("v1", "Pod", "payments", "api-7d9f-x2k"),
				ref("v1", "Secret", "payments", "db"),
			},
		}}
	}
	if err := c.Status().Update(ctx, app); err != nil {
		return err
	}
	if err := seedRevisions(ctx, c, a); err != nil {
		return err
	}
	return seedManaged(ctx, c, a)
}

func conditions(a sampleApp) []metav1.Condition {
	failing := a.health == corev1alpha1.HealthStateDegraded
	ready := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled", Message: "Live state matches desired state", LastTransitionTime: *ago(a.last)}
	switch {
	case failing:
		ready.Status, ready.Reason, ready.Message = metav1.ConditionFalse, "HealthFailure", "One or more resources are degraded"
	case a.sync == corev1alpha1.SyncStateAwaitingApproval:
		ready.Reason, ready.Message = "AwaitingApproval", "Plan sha256:a3f2c81907de waits for approval"
	}
	source := metav1.Condition{Type: "SourceReady", Status: metav1.ConditionTrue, Reason: "Resolved", Message: "Resolved " + a.commit, LastTransitionTime: *ago(a.last)}
	if a.repo == "finance-gitops" {
		source.Status, source.Reason, source.Message = metav1.ConditionFalse, "SourceFailure", "authentication required"
	}
	deps := metav1.Condition{Type: "DependenciesReady", Status: metav1.ConditionTrue, Reason: "NoDependencies", Message: "spec.dependsOn is empty", LastTransitionTime: *ago(96 * time.Hour)}
	return []metav1.Condition{ready, source, deps}
}

// plans holds the design's two non-trivial plans; every other newest
// Revision's plan leaves all resources unchanged.
func plan(a sampleApp) (corev1alpha1.RevisionPhase, corev1alpha1.RevisionPlan) {
	switch a.name {
	case "payments":
		return corev1alpha1.RevisionPhaseFailed, corev1alpha1.RevisionPlan{
			Digest:  "sha256:5b0e3c7a91c4",
			Summary: corev1alpha1.PlanSummary{Update: 2, Unchanged: 9},
			Resources: []corev1alpha1.PlanResourceChange{
				{Resource: ref("apps/v1", "Deployment", "payments", "api"), Action: corev1alpha1.PlanActionUpdate, Changes: []corev1alpha1.PlanFieldChange{
					{Path: "spec.template.spec.containers[api].image", Before: "ghcr.io/acme/payments:1.8.2", After: "ghcr.io/acme/payments:1.9.0"},
					{Path: "spec.template.spec.containers[api].envFrom[1].secretRef.name", After: "db"},
				}, Warnings: []string{"References Secret payments/db, which does not exist."}},
				{Resource: ref("v1", "ConfigMap", "payments", "api-config"), Action: corev1alpha1.PlanActionUpdate, Changes: []corev1alpha1.PlanFieldChange{
					{Path: "metadata.labels.log-level", Before: "info", After: "debug"},
				}},
				{Resource: ref("v1", "Secret", "payments", "stripe"), Action: corev1alpha1.PlanActionUnchanged, Warnings: []string{"Prune skipped: high-risk kind Secret."}},
			},
		}
	case "checkout":
		return corev1alpha1.RevisionPhaseAwaitingApproval, corev1alpha1.RevisionPlan{
			Digest:  "sha256:a3f2c81907de",
			Summary: corev1alpha1.PlanSummary{Create: 1, Update: 1, Delete: 1, Unchanged: 11},
			Resources: []corev1alpha1.PlanResourceChange{
				{Resource: ref("apps/v1", "Deployment", "shop", "checkout"), Action: corev1alpha1.PlanActionUpdate, Changes: []corev1alpha1.PlanFieldChange{{Path: "spec.replicas", Before: "3", After: "4"}}},
				{Resource: ref("autoscaling/v2", "HorizontalPodAutoscaler", "shop", "checkout"), Action: corev1alpha1.PlanActionCreate, Changes: []corev1alpha1.PlanFieldChange{{Path: "spec.maxReplicas", After: "10"}}},
				{Resource: ref("v1", "ConfigMap", "shop", "checkout-legacy"), Action: corev1alpha1.PlanActionDelete, Destructive: true, Warnings: []string{"Destructive: removed from desired state; prune is enabled."}},
			},
		}
	case "notifications":
		return corev1alpha1.RevisionPhaseApplying, corev1alpha1.RevisionPlan{Digest: "sha256:0c4e9d17a118", Summary: corev1alpha1.PlanSummary{Unchanged: 12}}
	default:
		return corev1alpha1.RevisionPhaseHealthy, corev1alpha1.RevisionPlan{Digest: "sha256:0c4e9d17a118", Summary: corev1alpha1.PlanSummary{Unchanged: 12}}
	}
}

// seedRevisions creates three Revisions per Application, oldest first, as
// the design's historyFor does.
func seedRevisions(ctx context.Context, c client.Client, a sampleApp) error {
	i := 0
	for k, s := range shas {
		if s == a.commit {
			i = k
		}
	}
	commits := []string{shas[(i+3)%5], shas[(i+2)%5], a.commit}
	started := []time.Duration{96 * time.Hour, 24 * time.Hour, a.last}
	for k, commit := range commits {
		rev := &corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: appNamespace,
				Name:      a.name + "-" + commit,
				Labels:    map[string]string{"sync.kuvryn.io/application": a.name},
			},
			Spec: corev1alpha1.RevisionSpec{
				ApplicationRef: corev1alpha1.LocalObjectReference{Name: a.name},
				Source: corev1alpha1.RevisionSource{
					RepositoryRef: corev1alpha1.LocalObjectReference{Name: a.repo},
					Revision:      fullSHA(commit),
					Path:          a.path,
					Render:        corev1alpha1.RenderSpec{Type: corev1alpha1.RenderType(a.render)},
				},
			},
		}
		if err := c.Create(ctx, rev); err != nil {
			return err
		}
		newest := k == len(commits)-1
		rev.Status = corev1alpha1.RevisionStatus{
			Phase:     corev1alpha1.RevisionPhaseHealthy,
			StartedAt: ago(started[k]),
			Attempts:  1,
			Plan:      corev1alpha1.RevisionPlan{Digest: fmt.Sprintf("sha256:%s%d", commit, k), Summary: corev1alpha1.PlanSummary{Update: int32(k + 1), Unchanged: 11}},
		}
		if newest {
			rev.Status.Phase, rev.Status.Plan = plan(a)
			if rev.Status.Phase == corev1alpha1.RevisionPhaseFailed {
				rev.Status.Attempts = 3
				rev.Status.Failure = &corev1alpha1.RevisionFailure{Reason: "HealthTimeout", Message: "Deployment payments/api did not become available"}
			}
		}
		if !a.auto && !newest {
			rev.Status.Approval = &corev1alpha1.RevisionApproval{ApprovedBy: "mia.chen@acme.io", ApprovedAt: *ago(started[k]), PlanDigest: rev.Status.Plan.Digest}
		}
		if err := c.Status().Update(ctx, rev); err != nil {
			return err
		}
	}
	return nil
}

// seedManaged creates the objects an Application manages in its
// destination namespace, labelled as the controller labels them.
func seedManaged(ctx context.Context, c client.Client, a sampleApp) error {
	name := a.name
	if a.name == "payments" {
		name = "api"
	}
	labels := map[string]string{"sync.kuvryn.io/application": a.name, "sync.kuvryn.io/application-namespace": appNamespace}
	meta := func(n string) metav1.ObjectMeta { return metav1.ObjectMeta{Namespace: a.ns, Name: n, Labels: labels} }
	selector := map[string]string{"app": name}
	replicas := int32(2)
	deploy := &appsv1.Deployment{
		ObjectMeta: meta(name),
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: "ghcr.io/acme/" + a.name + ":1.9.0"}}},
			},
		},
	}
	minAvailable := intstr.FromInt32(1)
	objs := []client.Object{
		deploy,
		&corev1.Service{ObjectMeta: meta(name), Spec: corev1.ServiceSpec{Selector: selector, Ports: []corev1.ServicePort{{Port: 80}}}},
		&corev1.ConfigMap{ObjectMeta: meta(name + "-config"), Data: map[string]string{"LOG_LEVEL": "info"}},
		&corev1.ServiceAccount{ObjectMeta: meta(name)},
		&policyv1.PodDisruptionBudget{ObjectMeta: meta(name), Spec: policyv1.PodDisruptionBudgetSpec{MinAvailable: &minAvailable, Selector: &metav1.LabelSelector{MatchLabels: selector}}},
	}
	for _, obj := range objs {
		if err := c.Create(ctx, obj); err != nil {
			return err
		}
	}
	// No controllers run in envtest, so record the Deployment's rollout.
	available := replicas
	if a.health == corev1alpha1.HealthStateDegraded || a.health == corev1alpha1.HealthStateProgressing {
		available = 0
	}
	deploy.Status = appsv1.DeploymentStatus{ObservedGeneration: deploy.Generation, Replicas: replicas, ReadyReplicas: available, AvailableReplicas: available, UpdatedReplicas: replicas}
	return c.Status().Update(ctx, deploy)
}
