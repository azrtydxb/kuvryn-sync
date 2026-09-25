#!/usr/bin/env bash
# Checks docs/migrate-flux.md end to end on a throwaway Kind cluster.
# See common.sh for the requirements.
C=solder-migrate-flux
# shellcheck source=hack/migration/common.sh
source "$(dirname "$0")/common.sh"
trap 'kind delete cluster --name "$C" >/dev/null 2>&1' EXIT
setup_solder
k apply -f https://github.com/fluxcd/flux2/releases/latest/download/install.yaml >/dev/null
k -n flux-system rollout status deploy/kustomize-controller deploy/source-controller --timeout=300s >/dev/null
cat <<Y | k apply -f - >/dev/null
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata: {name: fixture, namespace: flux-system}
spec: {interval: 1m, url: https://github.com/azrtydxb/solder-e2e-app.git, ref: {branch: main}}
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata: {name: fixture, namespace: flux-system}
spec: {interval: 1m, path: ./manifests, prune: true, sourceRef: {kind: GitRepository, name: fixture}}
Y
k create namespace solder-e2e >/dev/null
k -n flux-system wait kustomization/fixture --for=condition=Ready --timeout=300s >/dev/null
uid_before=$(k -n solder-e2e get configmap solder-e2e-config -o jsonpath='{.metadata.uid}')
echo "flux deployed configmap uid=$uid_before"
# Guide step 1.
k -n flux-system patch kustomization fixture --type merge -p '{"spec":{"suspend":true,"prune":false}}' >/dev/null
solder_migrate
# Guide step 5.
k -n flux-system delete kustomization fixture >/dev/null
sleep 20
clear_ownership sync.kuvryn.io/application=fixture
settle
verify kustomize-controller "$uid_before"
