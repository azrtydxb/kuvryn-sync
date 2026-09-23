# Shared steps for the migration checks in this directory. Each check creates
# a throwaway Kind cluster, installs cert-manager and Solder (image
# solder:e2e-local, built with `docker build -t solder:e2e-local .`), deploys
# the e2e fixture with Flux or Argo CD, then follows docs/migrate-*.md and
# verifies the workload survives and ends up owned by Solder alone:
#
#   bash hack/migration/migrate-flux.sh
#   bash hack/migration/migrate-argocd.sh
set -e
K="kubectl --context kind-$C"
setup_solder() {
	kind create cluster --name $C --wait 120s >/dev/null
	kind load docker-image solder:e2e-local --name $C >/dev/null
	$K apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml >/dev/null
	$K -n cert-manager rollout status deployment/cert-manager-webhook --timeout=180s >/dev/null
	for i in $(seq 1 60); do
		printf 'apiVersion: cert-manager.io/v1\nkind: Issuer\nmetadata: {name: probe, namespace: cert-manager}\nspec: {selfSigned: {}}\n' | $K apply --dry-run=server -f - >/dev/null 2>&1 && break
		sleep 2
	done
	$K apply -f config/crd/bases >/dev/null
	helm --kube-context kind-$C upgrade --install solder charts/solder -n solder-system --create-namespace \
		--set image.repository=solder --set image.tag=e2e-local --set replicaCount=1 --wait --timeout 180s >/dev/null
}
solder_migrate() {
	# Guide step 2 and 3, adapted to the fixture repository.
	$K -n solder-e2e create serviceaccount solder-e2e-deployer
	$K -n solder-e2e create rolebinding solder-e2e-deployer --clusterrole admin --serviceaccount solder-e2e:solder-e2e-deployer
	cat <<Y | $K apply -f - >/dev/null
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
	for i in $(seq 1 60); do
		phase=$($K -n solder-e2e get revision -l solder.io/application=fixture -o jsonpath='{.items[0].status.phase}' 2>/dev/null || true)
		[ "$phase" = AwaitingApproval ] && break
		sleep 3
	done
	rev=$($K -n solder-e2e get revision -l solder.io/application=fixture -o jsonpath='{.items[0].metadata.name}')
	echo "takeover in plan: $($K -n solder-e2e get revision $rev -o jsonpath='{.status.plan.resources[0].conflicts}')"
	$K -n solder-e2e annotate application.solder.io fixture solder.io/approved-revision=$rev >/dev/null
	for i in $(seq 1 60); do
		state=$($K -n solder-e2e get application.solder.io fixture -o jsonpath='{.status.sync.state}/{.status.health.state}')
		[ "$state" = Synced/Healthy ] && break
		sleep 3
	done
	echo "solder application: $state"
}
clear_ownership() {
	# Guide step 6.
	$K get all,configmap,secret,ingress -n solder-e2e -l "$1" -o name | xargs -I% $K -n solder-e2e patch % --type merge -p '{"metadata":{"managedFields":[{}]}}' >/dev/null
}
settle() {
	# Guide step 7.
	$K -n solder-e2e patch application.solder.io fixture --type merge -p '{"spec":{"sync":{"conflictPolicy":"fail","prune":true,"automatic":true}}}' >/dev/null
	sleep 15
	echo "after settling: $($K -n solder-e2e get application.solder.io fixture -o jsonpath='{.status.sync.state}/{.status.health.state}')"
}
verify() {
	prev_manager=$1
	uid_after=$($K -n solder-e2e get configmap solder-e2e-config -o jsonpath='{.metadata.uid}')
	echo "configmap kept (same UID): $([ "$uid_before" = "$uid_after" ] && echo yes || echo NO)"
	managers=$($K -n solder-e2e get configmap solder-e2e-config --show-managed-fields -o json | python3 -c 'import json,sys; o=json.load(sys.stdin); print(",".join(m["manager"] for m in o["metadata"].get("managedFields",[]) if "f:data" in json.dumps(m.get("fieldsV1",{}))))')
	echo "data owned by: $managers (previous: $prev_manager)"
}
