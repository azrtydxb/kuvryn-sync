package console

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/applier"
	"github.com/azrtydxb/kuvryn-sync/internal/graph"
	"github.com/azrtydxb/kuvryn-sync/internal/health"
)

// apiTimeout bounds every API call, cluster reads included.
const apiTimeout = 10 * time.Second

// sessionClearer ends a session, as *Auth does.
type sessionClearer interface{ ClearSession(w http.ResponseWriter) }

// apiHandler serves one read as the signed-in user.
type apiHandler func(ctx context.Context, r *http.Request, reader client.Reader) (any, error)

var (
	// errNeedNamespace marks a cluster-wide list the user may not make.
	errNeedNamespace = errors.New("console: a namespace is required")
	// errInvalidName marks a namespace or name no object can have.
	errInvalidName = errors.New("console: invalid name")
)

func (s *Server) registerAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/namespaces", s.api(s.namespaces))
	mux.HandleFunc("GET /api/applications", s.api(s.applications))
	mux.HandleFunc("GET /api/applications/{ns}/{name}", s.api(s.application))
	mux.HandleFunc("GET /api/applications/{ns}/{name}/revisions", s.api(s.applicationRevisions))
	mux.HandleFunc("GET /api/applications/{ns}/{name}/resources", s.api(s.applicationResources))
	mux.HandleFunc("GET /api/repositories", s.api(s.repositories))
	mux.HandleFunc("GET /api/revisions", s.api(s.revisions))
	mux.HandleFunc("GET /api/imagepolicies", s.api(s.imagePolicies))
}

// api signs the request in, builds the user's read-only client, bounds the
// call with apiTimeout, and maps cluster errors to JSON.
func (s *Server) api(h apiHandler) http.HandlerFunc {
	return s.withIdentity(func(w http.ResponseWriter, r *http.Request, id Identity) {
		ctx, cancel := context.WithTimeout(r.Context(), apiTimeout)
		defer cancel()
		reader, err := s.newReader(id)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		out, err := h(ctx, r, reader)
		if err != nil {
			s.writeError(ctx, w, r, id, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
}

func (s *Server) writeError(ctx context.Context, w http.ResponseWriter, r *http.Request, id Identity, err error) {
	switch {
	case errors.Is(err, errInvalidName):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid name"})
	case errors.Is(err, errNeedNamespace):
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden", "needNamespace": true})
	case apierrors.IsForbidden(err), errors.Is(err, ErrForbiddenPath), errors.Is(err, ErrWriteRefused), errors.Is(err, ErrNotImpersonated), errors.Is(err, ErrNotTheSessionToken):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case apierrors.IsNotFound(err):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "timeout"})
	case apierrors.IsUnauthorized(err):
		// The API server no longer accepts the session's credential: a
		// token expired or was revoked. End the session, so the browser's
		// return to the login page does not find it still signed in.
		if c, ok := s.auth.(sessionClearer); ok {
			c.ClearSession(w)
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	default:
		ctrllog.FromContext(r.Context()).Error(errors.New(scrub(err.Error(), id.Token.Reveal())), "Could not read the cluster", "path", r.URL.Path)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "cluster read failed"})
	}
}

// listIn lists into list in the ?namespace= namespace, or cluster-wide
// without one, turning a cluster-wide Forbidden into errNeedNamespace.
func listIn(ctx context.Context, r *http.Request, reader client.Reader, list client.ObjectList) error {
	ns := r.URL.Query().Get("namespace")
	err := reader.List(ctx, list, client.InNamespace(ns))
	if ns == "" && apierrors.IsForbidden(err) {
		return errNeedNamespace
	}
	return err
}

func (s *Server) namespaces(ctx context.Context, _ *http.Request, reader client.Reader) (any, error) {
	var list corev1.NamespaceList
	if err := reader.List(ctx, &list); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		out = append(out, ns.Name)
	}
	slices.Sort(out)
	return out, nil
}

func (s *Server) applications(ctx context.Context, r *http.Request, reader client.Reader) (any, error) {
	var list corev1alpha1.ApplicationList
	if err := listIn(ctx, r, reader, &list); err != nil {
		return nil, err
	}
	// One list of Revisions for every row. A viewer without list on
	// Revisions gets rows that fall back to their condition transitions; any
	// other failure is a read failure, not a reason to show stale times.
	var revs corev1alpha1.RevisionList
	newest := map[string]*corev1alpha1.Revision{}
	switch err := listIn(ctx, r, reader, &revs); {
	case err == nil:
		newest = newestByApplication(revs.Items)
	case apierrors.IsForbidden(err), errors.Is(err, errNeedNamespace):
	default:
		return nil, err
	}
	out := make([]AppRow, 0, len(list.Items))
	for i := range list.Items {
		app := &list.Items[i]
		out = append(out, appRow(app, newest[app.Namespace+"/"+app.Name]))
	}
	return out, nil
}

// belongsTo reports whether rev is one of the named Application's: labelled
// with its name, or naming it in spec.applicationRef.
func belongsTo(rev *corev1alpha1.Revision, app string) bool {
	return rev.Labels[applier.ApplicationLabelKey] == app || rev.Spec.ApplicationRef.Name == app
}

// newestByApplication maps namespace/name to each Application's newest
// Revision, by the same rule as revisionsOf.
func newestByApplication(revs []corev1alpha1.Revision) map[string]*corev1alpha1.Revision {
	newestFirst(revs)
	out := map[string]*corev1alpha1.Revision{}
	for i := range revs {
		rev := &revs[i]
		for _, app := range []string{rev.Labels[applier.ApplicationLabelKey], rev.Spec.ApplicationRef.Name} {
			key := rev.Namespace + "/" + app
			if _, seen := out[key]; app != "" && !seen {
				out[key] = rev
			}
		}
	}
	return out
}

// getApplication reads the Application the path names. A namespace or name
// no Application can have is refused before any request is built, so a
// crafted path such as ".." is a 400 rather than a client-go error.
func getApplication(ctx context.Context, r *http.Request, reader client.Reader) (*corev1alpha1.Application, error) {
	ns, name := r.PathValue("ns"), r.PathValue("name")
	if len(validation.IsDNS1123Label(ns)) > 0 || len(validation.IsDNS1123Subdomain(name)) > 0 {
		return nil, errInvalidName
	}
	app := &corev1alpha1.Application{}
	err := reader.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, app)
	return app, err
}

// revisionsOf returns app's Revisions, newest first: those labelled with its
// name and those whose spec.applicationRef names it.
func revisionsOf(ctx context.Context, reader client.Reader, app *corev1alpha1.Application) ([]corev1alpha1.Revision, error) {
	var list corev1alpha1.RevisionList
	if err := reader.List(ctx, &list, client.InNamespace(app.Namespace)); err != nil {
		return nil, err
	}
	out := []corev1alpha1.Revision{}
	for i := range list.Items {
		if belongsTo(&list.Items[i], app.Name) {
			out = append(out, list.Items[i])
		}
	}
	newestFirst(out)
	return out, nil
}

func (s *Server) application(ctx context.Context, r *http.Request, reader client.Reader) (any, error) {
	app, err := getApplication(ctx, r, reader)
	if err != nil {
		return nil, err
	}
	revs, err := revisionsOf(ctx, reader, app)
	if err != nil && !apierrors.IsForbidden(err) {
		return nil, err
	}
	var newest *corev1alpha1.Revision
	if len(revs) > 0 {
		newest = &revs[0]
	}
	return appDetail(app, newest, err == nil), nil
}

func (s *Server) applicationRevisions(ctx context.Context, r *http.Request, reader client.Reader) (any, error) {
	app, err := getApplication(ctx, r, reader)
	if err != nil {
		return nil, err
	}
	revs, err := revisionsOf(ctx, reader, app)
	if err != nil {
		return nil, err
	}
	out := make([]RevisionRow, 0, len(revs))
	for i := range revs {
		out = append(out, revisionRow(&revs[i]))
	}
	return out, nil
}

func isSecretKind(gvk schema.GroupVersionKind) bool { return gvk.Group == "" && gvk.Kind == "Secret" }

// applicationResources lists app's managed objects as the user. Secrets are
// never listed: each one the newest plan names becomes a row with only its
// name, and kinds the user may not list become rows that say so.
func (s *Server) applicationResources(ctx context.Context, r *http.Request, reader client.Reader) (any, error) {
	app, err := getApplication(ctx, r, reader)
	if err != nil {
		return nil, err
	}
	managed, skipped, err := applier.ListManaged(ctx, reader, app, applier.ListOptions{MetadataOnly: graph.IdentityOnly, Exclude: isSecretKind})
	if err != nil {
		return nil, err
	}
	var newest *corev1alpha1.Revision
	if revs, err := revisionsOf(ctx, reader, app); err == nil && len(revs) > 0 {
		newest = &revs[0]
	}
	pending := map[string]bool{}
	if newest != nil && newest.Status.Phase != corev1alpha1.RevisionPhaseHealthy {
		for _, res := range newest.Status.Plan.Resources {
			if res.Action != corev1alpha1.PlanActionUnchanged {
				pending[res.Resource.Kind+"/"+res.Resource.Name] = true
			}
		}
	}
	syncOf := func(kind, name string) string {
		switch {
		case app.Status.Sync.State == "":
			return stateUnknown
		case pending[kind+"/"+name]:
			return stateOutdated
		default:
			return stateSynced
		}
	}
	out := []ResourceRow{}
	for _, obj := range managed {
		row := ResourceRow{Kind: obj.GetKind(), Name: obj.GetName(), APIVersion: obj.GetAPIVersion(), Sync: syncOf(obj.GetKind(), obj.GetName()), Health: dash, Visible: true}
		if !graph.IdentityOnly(obj.GroupVersionKind()) {
			if res, err := health.Evaluate(obj); err == nil {
				row.Health = orUnknown(string(res.State))
			}
		}
		out = append(out, row)
	}
	for _, kind := range skipped {
		out = append(out, ResourceRow{Kind: kind, Name: dash, APIVersion: dash, Sync: dash, Health: dash})
	}
	if slices.ContainsFunc(managedKinds(app), isSecretKind) {
		names := secretNames(newest)
		if len(names) == 0 {
			names = []string{dash}
		}
		for _, name := range names {
			out = append(out, ResourceRow{Kind: "Secret", Name: name, APIVersion: "v1", Sync: dash, Health: dash})
		}
	}
	return out, nil
}

// managedKinds returns the kinds ListManaged would list for app.
func managedKinds(app *corev1alpha1.Application) []schema.GroupVersionKind {
	if len(app.Status.ManagedKinds) == 0 {
		return applier.DefaultKinds
	}
	return applier.InventoryKinds(app)
}

// secretNames returns the Secrets the newest plan names, never read from the
// cluster.
func secretNames(rev *corev1alpha1.Revision) []string {
	if rev == nil {
		return nil
	}
	names := []string{}
	for _, res := range rev.Status.Plan.Resources {
		if res.Resource.Kind == "Secret" && res.Resource.APIVersion == "v1" && res.Action != corev1alpha1.PlanActionDelete {
			names = append(names, res.Resource.Name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func (s *Server) repositories(ctx context.Context, r *http.Request, reader client.Reader) (any, error) {
	var list corev1alpha1.RepositoryList
	if err := listIn(ctx, r, reader, &list); err != nil {
		return nil, err
	}
	// Counting Applications needs list on them; without it the count is unknown.
	counts := map[string]int{}
	var apps corev1alpha1.ApplicationList
	countable := reader.List(ctx, &apps, client.InNamespace(r.URL.Query().Get("namespace"))) == nil
	for _, app := range apps.Items {
		counts[app.Namespace+"/"+app.Spec.Source.RepositoryRef.Name]++
	}
	out := make([]RepoRow, 0, len(list.Items))
	for i := range list.Items {
		var n *int
		if countable {
			c := counts[list.Items[i].Namespace+"/"+list.Items[i].Name]
			n = &c
		}
		out = append(out, repoRow(&list.Items[i], n))
	}
	return out, nil
}

func (s *Server) revisions(ctx context.Context, r *http.Request, reader client.Reader) (any, error) {
	var list corev1alpha1.RevisionList
	if err := listIn(ctx, r, reader, &list); err != nil {
		return nil, err
	}
	newestFirst(list.Items)
	out := make([]RevisionRow, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, revisionRow(&list.Items[i]))
	}
	return out, nil
}

func (s *Server) imagePolicies(ctx context.Context, r *http.Request, reader client.Reader) (any, error) {
	var list corev1alpha1.ImagePolicyList
	if err := listIn(ctx, r, reader, &list); err != nil {
		return nil, err
	}
	out := make([]ImagePolicyRow, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, imagePolicyRow(&list.Items[i]))
	}
	return out, nil
}
