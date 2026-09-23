C=solder-migrate-flux
source "$(dirname "$0")/common.sh"
trap 'kind delete cluster --name $C >/dev/null 2>&1' EXIT
setup_solder
$K apply -f https://github.com/fluxcd/flux2/releases/latest/download/install.yaml >/dev/null
$K -n flux-system rollout status deploy/kustomize-controller deploy/source-controller --timeout=300s >/dev/null
cat <<Y | $K apply -f - >/dev/null
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
$K create namespace solder-e2e >/dev/null
$K -n flux-system wait kustomization/fixture --for=condition=Ready --timeout=300s >/dev/null
uid_before=$($K -n solder-e2e get configmap solder-e2e-config -o jsonpath='{.metadata.uid}')
echo "flux deployed configmap uid=$uid_before"
# Guide step 1.
$K -n flux-system patch kustomization fixture --type merge -p '{"spec":{"suspend":true,"prune":false}}' >/dev/null
solder_migrate
# Guide step 5.
$K -n flux-system delete kustomization fixture >/dev/null
sleep 20
clear_ownership solder.io/application=fixture
settle
verify kustomize-controller
