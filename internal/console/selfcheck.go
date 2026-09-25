package console

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// SelfCheck asks the API server, with SelfSubjectAccessReviews, whether the
// console's own ServiceAccount may impersonate users and groups, logs the
// answer, and reports it on /healthz as "granted" or "missing". Without it
// every read the console makes is Forbidden. The reviews are the only
// requests the console sends as itself, and they read nothing. Without OIDC
// the console impersonates nobody, so it checks nothing and reports
// "disabled".
func (s *Server) SelfCheck(ctx context.Context) error {
	if !s.cfg.OIDCEnabled() {
		s.impersonation.Store("disabled")
		return nil
	}
	cs, err := kubernetes.NewForConfig(s.base)
	if err != nil {
		return err
	}
	log := ctrllog.FromContext(ctx)
	missing := []string{}
	for _, resource := range []string{"users", "groups"} {
		review := &authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{Verb: "impersonate", Group: "", Resource: resource},
		}}
		out, err := cs.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("check impersonate %s: %w", resource, err)
		}
		if !out.Status.Allowed {
			missing = append(missing, resource)
		}
	}
	if len(missing) > 0 {
		s.impersonation.Store("missing")
		log.Info("Console ServiceAccount may not impersonate; every read will be Forbidden until it is granted impersonate on users and groups", "missing", missing)
		return nil
	}
	s.impersonation.Store("granted")
	log.Info("Console ServiceAccount may impersonate users and groups")
	return nil
}
