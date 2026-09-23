package cli

import (
	"strings"
	"testing"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRenderCoreReadCommands(t *testing.T) {
	apps := RenderApplications([]corev1alpha1.Application{{ObjectMeta: metav1.ObjectMeta{Name: "payments"}, Status: corev1alpha1.ApplicationStatus{Sync: corev1alpha1.ApplicationSyncStatus{State: corev1alpha1.SyncStateSynced}, Health: corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateHealthy}, DesiredRevision: "abc", DeployedRevision: "abc", ServiceAccountName: "payments-deployer"}}})
	if !strings.Contains(apps, "payments") || !strings.Contains(apps, "Synced") || !strings.Contains(apps, "Healthy") || !strings.Contains(apps, "payments-deployer") || !strings.Contains(apps, "SERVICEACCOUNT") {
		t.Fatalf("bad app output: %s", apps)
	}
	repos := RenderRepositories([]corev1alpha1.Repository{{ObjectMeta: metav1.ObjectMeta{Name: "platform"}, Spec: corev1alpha1.RepositorySpec{Type: corev1alpha1.RepositoryTypeGit}, Status: corev1alpha1.RepositoryStatus{State: corev1alpha1.RepositoryStateReady, ObservedRevision: "abc"}}})
	if !strings.Contains(repos, "platform") || !strings.Contains(repos, "Ready") {
		t.Fatalf("bad repo output: %s", repos)
	}
}

func TestMutationPatchesAreSafeAndExact(t *testing.T) {
	app := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments"}}
	if _, err := BuildSyncPatch(app, "rev-a", "rev-b"); err == nil {
		t.Fatal("stale approval accepted")
	}
	patch, err := BuildSyncPatch(app, "rev-a", "rev-a")
	if err != nil {
		t.Fatal(err)
	}
	if patch.Resource != "applications.solder.io" || !strings.Contains(patch.Patch, "solder.io/approved-revision") {
		t.Fatalf("unexpected sync patch: %#v", patch)
	}
	patch, err = BuildSuspendPatch(app, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch.Patch, "suspend: true") {
		t.Fatalf("unexpected suspend patch: %#v", patch)
	}
}
