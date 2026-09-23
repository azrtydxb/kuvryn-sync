C=solder-migrate-argocd
source "$(dirname "$0")/common.sh"
trap 'kind delete cluster --name $C >/dev/null 2>&1' EXIT
setup_solder
$K create namespace argocd >/dev/null
$K apply -n argocd --server-side -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml >/dev/null
$K -n argocd rollout status statefulset/argocd-application-controller deploy/argocd-repo-server --timeout=600s >/dev/null
cat <<Y | $K apply -f - >/dev/null
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: fixture
  namespace: argocd
  finalizers: [resources-finalizer.argocd.argoproj.io]
spec:
  project: default
  source: {repoURL: https://github.com/azrtydxb/solder-e2e-app.git, targetRevision: main, path: manifests}
  destination: {server: https://kubernetes.default.svc, namespace: solder-e2e}
  syncPolicy: {automated: {prune: true, selfHeal: true}, syncOptions: [CreateNamespace=true]}
Y
for i in $(seq 1 100); do
  s=$($K -n argocd get application fixture -o jsonpath='{.status.sync.status}/{.status.health.status}' 2>/dev/null || true)
  [ "$s" = Synced/Healthy ] && break; sleep 3
done
echo "argo cd: $s"
uid_before=$($K -n solder-e2e get configmap solder-e2e-config -o jsonpath='{.metadata.uid}')
echo "argo cd deployed configmap uid=$uid_before"
# Guide step 1.
$K -n argocd patch application fixture --type json -p '[{"op":"remove","path":"/spec/syncPolicy/automated"}]' >/dev/null
$K -n argocd patch application fixture --type json -p '[{"op":"remove","path":"/metadata/finalizers"}]' >/dev/null
solder_migrate
# Guide step 5.
$K -n argocd delete application fixture >/dev/null
sleep 20
clear_ownership solder.io/application=fixture
settle
verify argocd-controller
