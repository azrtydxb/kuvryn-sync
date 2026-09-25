#!/usr/bin/env bash
# Checks docs/migrate-argocd.md end to end on a throwaway Kind cluster.
# See common.sh for the requirements.
C=kuvryn-sync-migrate-argocd
# shellcheck source=hack/migration/common.sh
source "$(dirname "$0")/common.sh"
trap 'kind delete cluster --name "$C" >/dev/null 2>&1' EXIT
setup_ksync
k create namespace argocd >/dev/null
k apply -n argocd --server-side -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml >/dev/null
k -n argocd rollout status statefulset/argocd-application-controller deploy/argocd-repo-server --timeout=600s >/dev/null
cat <<Y | k apply -f - >/dev/null
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: fixture
  namespace: argocd
  finalizers: [resources-finalizer.argocd.argoproj.io]
spec:
  project: default
  source: {repoURL: https://github.com/azrtydxb/kuvryn-sync-e2e-app.git, targetRevision: main, path: manifests}
  destination: {server: https://kubernetes.default.svc, namespace: kuvryn-sync-e2e}
  syncPolicy: {automated: {prune: true, selfHeal: true}, syncOptions: [CreateNamespace=true]}
Y
s=""
for _ in $(seq 1 100); do
	s=$(k -n argocd get applications.argoproj.io fixture -o jsonpath='{.status.sync.status}/{.status.health.status}' 2>/dev/null || true)
	[ "$s" = Synced/Healthy ] && break
	sleep 3
done
[ "$s" = Synced/Healthy ] || fail "Argo CD Application did not become Synced/Healthy (last state: $s)"
echo "argo cd: $s"
uid_before=$(k -n kuvryn-sync-e2e get configmap kuvryn-sync-e2e-config -o jsonpath='{.metadata.uid}')
echo "argo cd deployed configmap uid=$uid_before"
# Guide step 1.
k -n argocd patch applications.argoproj.io fixture --type json -p '[{"op":"remove","path":"/spec/syncPolicy/automated"}]' >/dev/null
k -n argocd patch applications.argoproj.io fixture --type json -p '[{"op":"remove","path":"/metadata/finalizers"}]' >/dev/null
ksync_migrate
# Guide step 5.
k -n argocd delete applications.argoproj.io fixture >/dev/null
sleep 20
clear_ownership sync.kuvryn.io/application=fixture
settle
verify argocd-controller "$uid_before"
