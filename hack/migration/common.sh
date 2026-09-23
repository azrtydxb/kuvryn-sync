# shellcheck shell=bash
# Shared steps for the migration checks in this directory. Each check creates
# a throwaway Kind cluster, installs cert-manager and Solder (image
# solder:e2e-local, built with `docker build -t solder:e2e-local .`), deploys
# the e2e fixture with Flux or Argo CD, then follows docs/migrate-*.md and
# verifies the workload survives and ends up owned by Solder alone:
#
#   bash hack/migration/migrate-flux.sh
#   bash hack/migration/migrate-argocd.sh
#
# Requires kind, kubectl, helm, docker and python3 (used to read
# managedFields). Callers set C to the Kind cluster name before sourcing this
# file. Every check exits non-zero when a step times out or the result is not
# what the guide promises.
set -euo pipefail

# The field manager Solder applies with (internal/applier.FieldManager).
SOLDER_FIELD_MANAGER=solder

k() { kubectl --context "kind-$C" "$@"; }

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

setup_solder() {
	kind create cluster --name "$C" --wait 120s >/dev/null
	kind load docker-image solder:e2e-local --name "$C" >/dev/null
	k apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml >/dev/null
	k -n cert-manager rollout status deployment/cert-manager-webhook --timeout=180s >/dev/null
	local ready=""
	for _ in $(seq 1 60); do
		if printf 'apiVersion: cert-manager.io/v1\nkind: Issuer\nmetadata: {name: probe, namespace: cert-manager}\nspec: {selfSigned: {}}\n' |
			k apply --dry-run=server -f - >/dev/null 2>&1; then
			ready=yes
			break
		fi
		sleep 2
	done
	[ -n "$ready" ] || fail "cert-manager webhook did not become ready"
	k apply -f config/crd/bases >/dev/null
	helm --kube-context "kind-$C" upgrade --install solder charts/solder -n solder-system --create-namespace \
		--set image.repository=solder --set image.tag=e2e-local --set replicaCount=1 --wait --timeout 180s >/dev/null
}

solder_migrate() {
	# Guide step 2 and 3, adapted to the fixture repository.
	k -n solder-e2e create serviceaccount solder-e2e-deployer
	k -n solder-e2e create rolebinding solder-e2e-deployer --clusterrole admin --serviceaccount solder-e2e:solder-e2e-deployer
	cat <<Y | k apply -f - >/dev/null
apiVersion: solder.io/v1alpha1
kind: Repository
metadata: {name: platform, namespace: solder-e2e}
spec: {git: {url: https://github.com/azrtydxb/solder-e2e-app.git, revision: main}}
---
apiVersion: solder.io/v1alpha1
kind: Application
metadata: {name: fixture, namespace: solder-e2e}
spec:
  serviceAccountName: solder-e2e-deployer
  source: {repositoryRef: {name: platform}, path: manifests, render: {type: yaml}}
  destination: {namespace: solder-e2e}
  sync: {automatic: false, prune: false, conflictPolicy: adopt}
Y
	# Guide step 4: review, then approve.
	local phase=""
	for _ in $(seq 1 60); do
		phase=$(k -n solder-e2e get revision -l solder.io/application=fixture -o jsonpath='{.items[0].status.phase}' 2>/dev/null || true)
		[ "$phase" = AwaitingApproval ] && break
		sleep 3
	done
	[ "$phase" = AwaitingApproval ] || fail "Revision did not reach AwaitingApproval (last phase: ${phase:-none})"
	local rev
	rev=$(k -n solder-e2e get revision -l solder.io/application=fixture -o jsonpath='{.items[0].metadata.name}')
	[ -n "$rev" ] || fail "no Revision found for Application fixture"
	echo "takeover in plan: $(k -n solder-e2e get revision "$rev" -o jsonpath='{.status.plan.resources[0].conflicts}')"
	k -n solder-e2e annotate applications.solder.io fixture "solder.io/approved-revision=$rev" >/dev/null
	local state=""
	for _ in $(seq 1 60); do
		state=$(k -n solder-e2e get applications.solder.io fixture -o jsonpath='{.status.sync.state}/{.status.health.state}' 2>/dev/null || true)
		[ "$state" = Synced/Healthy ] && break
		sleep 3
	done
	[ "$state" = Synced/Healthy ] || fail "Solder Application did not become Synced/Healthy (last state: $state)"
	echo "solder application: $state"
}

clear_ownership() {
	# Guide step 6.
	local obj
	k get all,configmap,secret,ingress -n solder-e2e -l "$1" -o name | while read -r obj; do
		k -n solder-e2e patch "$obj" --type merge -p '{"metadata":{"managedFields":[{}]}}' >/dev/null
	done
}

settle() {
	# Guide step 7.
	k -n solder-e2e patch applications.solder.io fixture --type merge -p '{"spec":{"sync":{"conflictPolicy":"fail","prune":true,"automatic":true}}}' >/dev/null
	# Give the controller time to act on the new policy before polling, so a
	# stale Synced/Healthy status does not pass the check.
	sleep 15
	local state=""
	for _ in $(seq 1 20); do
		state=$(k -n solder-e2e get applications.solder.io fixture -o jsonpath='{.status.sync.state}/{.status.health.state}' 2>/dev/null || true)
		[ "$state" = Synced/Healthy ] && break
		sleep 3
	done
	[ "$state" = Synced/Healthy ] || fail "Solder Application did not settle to Synced/Healthy (last state: $state)"
	echo "after settling: $state"
}

verify() {
	local prev_manager=$1 uid_before=$2 uid_after managers
	uid_after=$(k -n solder-e2e get configmap solder-e2e-config -o jsonpath='{.metadata.uid}')
	[ "$uid_before" = "$uid_after" ] || fail "configmap was recreated (UID $uid_before became $uid_after)"
	echo "configmap kept (same UID): yes"
	managers=$(k -n solder-e2e get configmap solder-e2e-config --show-managed-fields -o json |
		python3 -c 'import json,sys; o=json.load(sys.stdin); print(",".join(m["manager"] for m in o["metadata"].get("managedFields",[]) if "f:data" in json.dumps(m.get("fieldsV1",{}))))')
	echo "data owned by: $managers (previous: $prev_manager)"
	[ "$managers" = "$SOLDER_FIELD_MANAGER" ] || fail "configmap data is owned by '$managers', not by $SOLDER_FIELD_MANAGER alone"
	echo "PASS"
}
