package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/client-go/rest"
)

// fakeSSARServer answers SelfSubjectAccessReviews, allowing impersonation of
// the resources in allowed.
func fakeSSARServer(t *testing.T, allowed ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" {
			http.NotFound(w, r)
			return
		}
		var review authorizationv1.SelfSubjectAccessReview
		if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
			t.Error(err)
		}
		attrs := review.Spec.ResourceAttributes
		for _, a := range allowed {
			if attrs != nil && attrs.Verb == "impersonate" && attrs.Group == "" && attrs.Resource == a {
				review.Status.Allowed = true
			}
		}
		review.APIVersion, review.Kind = "authorization.k8s.io/v1", "SelfSubjectAccessReview"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(review)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSelfCheckReportsImpersonationOnHealthz(t *testing.T) {
	for _, tc := range []struct {
		allowed []string
		want    string
	}{
		{[]string{"users", "groups"}, `"impersonation":"granted"`},
		{[]string{"users"}, `"impersonation":"missing"`},
		{nil, `"impersonation":"missing"`},
	} {
		api := fakeSSARServer(t, tc.allowed...)
		s, err := NewServer(Config{}, &rest.Config{Host: api.URL, ContentConfig: rest.ContentConfig{ContentType: "application/json"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SelfCheck(t.Context()); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("allowed %v: healthz = %s, want %s", tc.allowed, rec.Body.String(), tc.want)
		}
	}
}
