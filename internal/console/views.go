package console

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/redact"
)

// dash is shown for every unknown value.
const dash = "—"

const (
	stateUnknown  = "Unknown"
	stateSynced   = "Synced"
	stateOutdated = "OutOfSync"
)

// AppRow is one Application in the list. LastChange is the latest of its
// condition transitions and its newest Revision's start and completion, and
// is what the console shows; LastReconcile, the newest condition transition
// alone, is kept for API compatibility.
type AppRow struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Destination   string `json:"destination"`
	Repository    string `json:"repository"`
	Path          string `json:"path"`
	Render        string `json:"render"`
	Commit        string `json:"commit"`
	Sync          string `json:"sync"`
	Health        string `json:"health"`
	LastReconcile string `json:"lastReconcile"`
	LastChange    string `json:"lastChange"`
}

// SourceView is an Application's source.
type SourceView struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Path       string `json:"path"`
	Render     string `json:"render"`
}

// PolicyView is an Application's sync policy.
type PolicyView struct {
	Automatic          bool   `json:"automatic"`
	Prune              bool   `json:"prune"`
	SelfHeal           bool   `json:"selfHeal"`
	Suspend            bool   `json:"suspend"`
	ConflictPolicy     string `json:"conflictPolicy"`
	FailureAction      string `json:"failureAction"`
	DeletionPolicy     string `json:"deletionPolicy"`
	ServiceAccountName string `json:"serviceAccountName"`
}

// ConditionView is one status condition.
type ConditionView struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
	LastTransitionTime string `json:"lastTransitionTime"`
}

// ChainLink is one resource on the way from an unhealthy managed object to
// its root cause. Only the root carries a state: the cause's reason.
type ChainLink struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// Cause is one recorded diagnosis cause.
type Cause struct {
	Resource string      `json:"resource"`
	Reason   string      `json:"reason"`
	Message  string      `json:"message"`
	Chain    []ChainLink `json:"chain"`
}

// RefView identifies a resource.
type RefView struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
}

// ChangeView is one planned field change.
type ChangeView struct {
	Path     string `json:"path"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Redacted bool   `json:"redacted"`
}

// PlanResourceView is one resource in a plan.
type PlanResourceView struct {
	Action   string       `json:"action"`
	Ref      RefView      `json:"ref"`
	Changes  []ChangeView `json:"changes"`
	Warnings []string     `json:"warnings"`
}

// SummaryView counts a plan's actions.
type SummaryView struct {
	Create    int32 `json:"create"`
	Update    int32 `json:"update"`
	Delete    int32 `json:"delete"`
	Unchanged int32 `json:"unchanged"`
}

// PlanView is the newest Revision's plan.
type PlanView struct {
	Revision  string             `json:"revision"`
	Commit    string             `json:"commit"`
	Digest    string             `json:"digest"`
	Phase     string             `json:"phase"`
	Summary   SummaryView        `json:"summary"`
	Truncated bool               `json:"truncated"`
	Resources []PlanResourceView `json:"resources"`
}

// AppDetail is one Application with everything its tabs show.
type AppDetail struct {
	AppRow
	State            string          `json:"state"`
	DesiredRevision  string          `json:"desiredRevision"`
	DeployedRevision string          `json:"deployedRevision"`
	Source           SourceView      `json:"source"`
	Policy           PolicyView      `json:"policy"`
	Conditions       []ConditionView `json:"conditions"`
	Diagnosis        []Cause         `json:"diagnosis"`
	Plan             *PlanView       `json:"plan"`
	// PlanVisible is false when the user may not list Revisions.
	PlanVisible bool `json:"planVisible"`
}

// RevisionRow is one Revision.
type RevisionRow struct {
	Name        string      `json:"name"`
	Namespace   string      `json:"namespace"`
	Application string      `json:"application"`
	Commit      string      `json:"commit"`
	Phase       string      `json:"phase"`
	Plan        SummaryView `json:"plan"`
	Digest      string      `json:"digest"`
	ApprovedBy  string      `json:"approvedBy"`
	Attempts    int32       `json:"attempts"`
	Started     string      `json:"started"`
	Failure     string      `json:"failure"`
}

// ResourceRow is one managed object. Rows the user may not read, and every
// Secret, have visible false and carry no content.
type ResourceRow struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	APIVersion string `json:"apiVersion"`
	Sync       string `json:"sync"`
	Health     string `json:"health"`
	Visible    bool   `json:"visible"`
}

// RepoRow is one Repository.
type RepoRow struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	URL       string `json:"url"`
	Ref       string `json:"ref"`
	Observed  string `json:"observed"`
	State     string `json:"state"`
	Message   string `json:"message"`
	Apps      *int   `json:"apps"`
	Poll      string `json:"poll"`
	Webhook   bool   `json:"webhook"`
	LastFetch string `json:"lastFetch"`
}

// ImagePolicyRow is one ImagePolicy.
type ImagePolicyRow struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Rule      string `json:"rule"`
	Latest    string `json:"latest"`
	Digest    string `json:"digest"`
	LastScan  string `json:"lastScan"`
}

func orDash(s string) string {
	if s == "" {
		return dash
	}
	return s
}

func orUnknown(s string) string {
	if s == "" {
		return stateUnknown
	}
	return s
}

func timeOrDash(t *metav1.Time) string {
	if t == nil || t.IsZero() {
		return dash
	}
	return t.UTC().Format(time.RFC3339)
}

// text redacts a message for display.
func text(s string) string { return orDash(redact.String(s)) }

// appRow is app's list row. newest is its newest Revision, or nil when it
// has none or the user may not list Revisions.
func appRow(app *corev1alpha1.Application, newest *corev1alpha1.Revision) AppRow {
	var transition *metav1.Time
	for i := range app.Status.Conditions {
		if t := &app.Status.Conditions[i].LastTransitionTime; transition == nil || transition.Before(t) {
			transition = t
		}
	}
	return AppRow{
		Name:          app.Name,
		Namespace:     app.Namespace,
		Destination:   app.DestinationNamespace(),
		Repository:    orDash(app.Spec.Source.RepositoryRef.Name),
		Path:          orDash(app.Spec.Source.Path),
		Render:        orDash(string(app.Spec.Source.Render.Type)),
		Commit:        orDash(app.Status.DeployedRevision),
		Sync:          orUnknown(string(app.Status.Sync.State)),
		Health:        orUnknown(string(app.Status.Health.State)),
		LastReconcile: timeOrDash(transition),
		LastChange:    timeOrDash(lastChange(transition, newest)),
	}
}

// lastChange is the latest of an Application's newest condition transition
// and its newest Revision's start and completion. Ready stays True across
// deploys, so the transition alone goes stale.
func lastChange(transition *metav1.Time, newest *corev1alpha1.Revision) *metav1.Time {
	last := transition
	later := func(t *metav1.Time) {
		if t != nil && !t.IsZero() && (last == nil || last.Before(t)) {
			last = t
		}
	}
	if newest != nil {
		started := metav1.NewTime(startOf(newest))
		later(&started)
		later(newest.Status.CompletedAt)
	}
	return last
}

func appDetail(app *corev1alpha1.Application, newest *corev1alpha1.Revision, planVisible bool) AppDetail {
	d := AppDetail{
		AppRow:           appRow(app, newest),
		State:            orUnknown(string(app.Status.State)),
		DesiredRevision:  orDash(app.Status.DesiredRevision),
		DeployedRevision: orDash(app.Status.DeployedRevision),
		Source: SourceView{
			Repository: orDash(app.Spec.Source.RepositoryRef.Name),
			Revision:   orDash(app.Spec.Source.Revision),
			Path:       orDash(app.Spec.Source.Path),
			Render:     orDash(string(app.Spec.Source.Render.Type)),
		},
		Policy: PolicyView{
			Automatic:          app.Spec.Sync.Automatic,
			Prune:              app.Spec.Sync.Prune,
			SelfHeal:           app.Spec.Sync.SelfHeal,
			Suspend:            app.Spec.Suspend,
			ConflictPolicy:     orDash(string(app.Spec.Sync.ConflictPolicy)),
			FailureAction:      orDash(string(app.Spec.Strategy.FailurePolicy.Action)),
			DeletionPolicy:     orDash(string(app.Spec.DeletionPolicy)),
			ServiceAccountName: orDash(cmp.Or(app.Status.ServiceAccountName, app.Spec.ServiceAccountName)),
		},
		Conditions:  []ConditionView{},
		Diagnosis:   causes(app.Status.Diagnosis),
		PlanVisible: planVisible,
	}
	for _, c := range app.Status.Conditions {
		d.Conditions = append(d.Conditions, ConditionView{
			Type:               c.Type,
			Status:             string(c.Status),
			Reason:             orDash(c.Reason),
			Message:            text(c.Message),
			LastTransitionTime: timeOrDash(&c.LastTransitionTime),
		})
	}
	if newest != nil {
		d.Plan = planView(newest)
	}
	return d
}

func causes(in []corev1alpha1.DiagnosisCause) []Cause {
	out := make([]Cause, 0, len(in))
	for _, c := range in {
		chain := make([]ChainLink, 0, len(c.Chain))
		for _, ref := range c.Chain {
			state := dash
			if ref == c.Resource {
				state = orDash(c.Reason)
			}
			chain = append(chain, ChainLink{Kind: ref.Kind, Name: qualified(ref), State: state})
		}
		out = append(out, Cause{
			Resource: c.Resource.Kind + "/" + qualified(c.Resource),
			Reason:   orDash(c.Reason),
			Message:  text(c.Message),
			Chain:    chain,
		})
	}
	return out
}

// qualified is a reference's namespace/name, or its name when cluster-scoped.
func qualified(ref corev1alpha1.ResourceRef) string {
	if ref.Namespace == "" {
		return ref.Name
	}
	return ref.Namespace + "/" + ref.Name
}

func summaryView(s corev1alpha1.PlanSummary) SummaryView {
	return SummaryView{Create: s.Create, Update: s.Update, Delete: s.Delete, Unchanged: s.Unchanged}
}

func planView(rev *corev1alpha1.Revision) *PlanView {
	p := &PlanView{
		Revision:  rev.Name,
		Commit:    orDash(rev.Spec.Source.Revision),
		Digest:    orDash(rev.Status.Plan.Digest),
		Phase:     orUnknown(string(rev.Status.Phase)),
		Summary:   summaryView(rev.Status.Plan.Summary),
		Truncated: rev.Status.Plan.Summary.Truncated,
		Resources: []PlanResourceView{},
	}
	for _, r := range rev.Status.Plan.Resources {
		view := PlanResourceView{
			Action:   string(r.Action),
			Ref:      RefView{APIVersion: r.Resource.APIVersion, Kind: r.Resource.Kind, Namespace: r.Resource.Namespace, Name: r.Resource.Name},
			Changes:  []ChangeView{},
			Warnings: []string{},
		}
		for _, c := range r.Changes {
			change := ChangeView{Path: c.Path, Before: redact.String(c.Before), After: redact.String(c.After)}
			// The same rule as ksync plan's output.
			if r.Resource.Kind == "Secret" || c.Redacted || redact.SensitivePath(c.Path) {
				change.Before, change.After = redact.Value(c.Before), redact.Value(c.After)
				change.Redacted = true
			}
			view.Changes = append(view.Changes, change)
		}
		for _, w := range r.Warnings {
			view.Warnings = append(view.Warnings, redact.String(w))
		}
		p.Resources = append(p.Resources, view)
	}
	return p
}

func revisionRow(rev *corev1alpha1.Revision) RevisionRow {
	row := RevisionRow{
		Name:        rev.Name,
		Namespace:   rev.Namespace,
		Application: orDash(rev.Spec.ApplicationRef.Name),
		Commit:      orDash(rev.Spec.Source.Revision),
		Phase:       orUnknown(string(rev.Status.Phase)),
		Plan:        summaryView(rev.Status.Plan.Summary),
		Digest:      orDash(rev.Status.Plan.Digest),
		ApprovedBy:  dash,
		Attempts:    rev.Status.Attempts,
		Started:     timeOrDash(rev.Status.StartedAt),
		Failure:     dash,
	}
	if rev.Status.StartedAt == nil {
		row.Started = timeOrDash(&rev.CreationTimestamp)
	}
	if a := rev.Status.Approval; a != nil {
		row.ApprovedBy = orDash(a.ApprovedBy)
	}
	if f := rev.Status.Failure; f != nil {
		row.Failure = text(strings.Join(slices.DeleteFunc([]string{f.Reason, f.Message}, func(s string) bool { return s == "" }), ": "))
	}
	return row
}

// startOf is when a Revision started, or its creation before it has.
func startOf(rev *corev1alpha1.Revision) time.Time {
	if rev.Status.StartedAt != nil {
		return rev.Status.StartedAt.Time
	}
	return rev.CreationTimestamp.Time
}

// newestFirst sorts Revisions newest first by start, then by name.
// Creation timestamps have one-second resolution, so Revisions created
// together would otherwise sort by name alone.
func newestFirst(revs []corev1alpha1.Revision) {
	slices.SortStableFunc(revs, func(a, b corev1alpha1.Revision) int {
		if c := startOf(&b).Compare(startOf(&a)); c != 0 {
			return c
		}
		return cmp.Compare(b.Name, a.Name)
	})
}

// shortDuration formats a poll interval as 90s, 5m or 2h.
func shortDuration(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	case d >= time.Minute && d%time.Minute == 0:
		return strconv.FormatInt(int64(d/time.Minute), 10) + "m"
	default:
		return strconv.FormatInt(int64(d.Round(time.Second)/time.Second), 10) + "s"
	}
}

func repoRow(repo *corev1alpha1.Repository, apps *int) RepoRow {
	row := RepoRow{
		Name:      repo.Name,
		Namespace: repo.Namespace,
		URL:       dash,
		Ref:       dash,
		Observed:  orDash(repo.Status.ObservedRevision),
		State:     orUnknown(string(repo.Status.State)),
		Message:   dash,
		Apps:      apps,
		Poll:      dash,
		Webhook:   repo.Spec.Webhook != nil,
		LastFetch: timeOrDash(repo.Status.LastFetchedAt),
	}
	if g := repo.Spec.Git; g != nil {
		row.URL = text(g.URL)
		row.Ref = orDash(g.Revision)
	}
	if repo.Spec.PollInterval != nil {
		row.Poll = shortDuration(repo.Spec.PollInterval.Duration)
	}
	for _, c := range repo.Status.Conditions {
		if c.Type == "Ready" {
			row.Message = text(c.Message)
		}
	}
	return row
}

func imagePolicyRow(p *corev1alpha1.ImagePolicy) ImagePolicyRow {
	rule := dash
	switch pol := p.Spec.Policy; {
	case pol.Semver != nil:
		rule = "semver " + pol.Semver.Range
	case pol.TagPattern != nil:
		rule = "tagPattern " + pol.TagPattern.Regex
	case pol.Digest != nil:
		rule = "digest " + pol.Digest.Tag
	}
	return ImagePolicyRow{
		Name:      p.Name,
		Namespace: p.Namespace,
		Image:     text(p.Spec.Image),
		Rule:      rule,
		Latest:    orDash(p.Status.LatestTag),
		Digest:    orDash(p.Status.LatestDigest),
		LastScan:  timeOrDash(p.Status.LastScannedAt),
	}
}
