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

package source

import "context"

// FailureReason classifies source failures for status and Events.
type FailureReason string

const (
	FailureReasonSourceFailure         FailureReason = "SourceFailure"
	FailureReasonAuthenticationFailure FailureReason = "AuthenticationFailure"
	FailureReasonValidationFailure     FailureReason = "ValidationFailure"
)

// Error is a classified source error safe to expose in status and Events.
type Error struct {
	Reason  FailureReason
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Credentials contains Git credential material loaded from Kubernetes Secrets.
// Values must not be logged, written to status, or included in Events.
type Credentials struct {
	Username   string
	Password   string
	Token      string
	SSHKey     string
	KnownHosts string
}

// GitRepository identifies a Git desired-state source.
type GitRepository struct {
	URL      string
	Revision string
	Auth     Credentials
}

// ResolvedSource is an immutable source revision observed by Kuvryn Sync.
type ResolvedSource struct {
	Revision string
	CacheDir string
}

// Resolver resolves a desired-state source reference to an immutable revision.
type Resolver interface {
	Resolve(ctx context.Context, repository GitRepository) (ResolvedSource, error)
}
