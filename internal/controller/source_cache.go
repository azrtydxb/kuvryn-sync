/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"cmp"
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	gitcache "github.com/azrtydxb/solder/internal/source/git"
)

const (
	defaultCachePruneInterval = time.Hour
	// defaultCacheGracePeriod covers renders still reading a checkout that
	// was resolved moments ago but is not yet recorded on a Revision.
	defaultCacheGracePeriod = time.Hour
)

// NewSourceCache returns the Git cache the Repository and Application
// controllers share, so one lock guards each cached repository.
func NewSourceCache(dir string) *gitcache.Cache {
	return gitcache.NewCache(cacheRoot(dir))
}

// SourceCachePruner periodically removes Git checkouts that no Revision or
// Repository refers to any more. Each replica has its own cache on local
// disk, so every replica prunes, leader or not.
type SourceCachePruner struct {
	Client      client.Reader
	Cache       *gitcache.Cache
	Interval    time.Duration
	GracePeriod time.Duration
}

// NeedLeaderElection is false: the cache is per replica.
func (p *SourceCachePruner) NeedLeaderElection() bool { return false }

// Start prunes every interval until ctx is done.
func (p *SourceCachePruner) Start(ctx context.Context) error {
	ticker := time.NewTicker(cmp.Or(p.Interval, defaultCachePruneInterval))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := p.prune(ctx); err != nil {
				logf.FromContext(ctx).Error(err, "Failed to prune the Git source cache")
			}
		}
	}
}

// prune removes checkouts no Revision or Repository still needs.
func (p *SourceCachePruner) prune(ctx context.Context) error {
	keep, err := keptCommits(ctx, p.Client)
	if err != nil {
		return err
	}
	removed, err := p.Cache.Prune(keep, time.Now().Add(-cmp.Or(p.GracePeriod, defaultCacheGracePeriod)))
	if len(removed) > 0 {
		logf.FromContext(ctx).Info("Pruned Git source cache", "removed", len(removed))
	}
	return err
}

// keptCommits maps each Repository's URL to the commits that must stay
// checked out: its latest observed commit and every commit an existing
// Revision was rendered from.
func keptCommits(ctx context.Context, c client.Reader) (map[string][]string, error) {
	repositories := &corev1alpha1.RepositoryList{}
	if err := c.List(ctx, repositories); err != nil {
		return nil, err
	}
	urls := map[client.ObjectKey]string{}
	keep := map[string][]string{}
	for _, repository := range repositories.Items {
		if repository.Spec.Git == nil {
			continue
		}
		url := repository.Spec.Git.URL
		urls[client.ObjectKeyFromObject(&repository)] = url
		keep[url] = append(keep[url], repository.Status.ObservedRevision)
	}
	revisions := &corev1alpha1.RevisionList{}
	if err := c.List(ctx, revisions); err != nil {
		return nil, err
	}
	for _, revision := range revisions.Items {
		url, ok := urls[client.ObjectKey{Namespace: revision.Namespace, Name: revision.Spec.Source.RepositoryRef.Name}]
		if ok {
			keep[url] = append(keep[url], revision.Spec.Source.Revision)
		}
	}
	return keep, nil
}
