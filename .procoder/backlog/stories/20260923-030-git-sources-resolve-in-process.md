# Git sources resolve in process

Status: done 2026-09-23
Created: 2026-09-23
Epic: in-process-sources-and-renderers
Sprint: -

## Description

As an operator, Repository sources resolve with go-git, so the controller image needs no git binary, and a checkout can never place symlinks that point outside it.

## Acceptance criteria

- [x] The source cache uses go-git for fetch, ref resolution (branch, tag, full or short commit, default HEAD), and worktree materialisation; the Dockerfile no longer installs git.
- [x] HTTPS basic/token auth and SSH key auth work; SSH requires known_hosts and rejects unknown host keys.
- [x] Materialisation refuses symlinks whose target resolves outside the worktree; a unit test proves it.
- [x] Existing cache tests pass against local repositories created in the test.

## Evidence

- `internal/source/git` uses go-git v5.19.2 for fetch (heads and tags, prune), resolution (`ResolveRevision`: branch, tag incl. annotated, full or short commit; HEAD via the remote's symbolic HEAD), and worktree materialisation into a staging dir renamed into place. The Dockerfile no longer installs git; the built image has no git, helm, or kustomize binary.
- Auth: HTTPS basic/token via go-git BasicAuth; SSH via public keys with a known_hosts callback, and `TestSSHRepositoryWithoutKnownHostsFailsAuthentication` proves SSH without known_hosts fails as AuthenticationFailure. Filesystem repository URLs are rejected (`TestLocalRepositoryURLsAreRejected`); go-git's local transport would need git binaries and a controller should not read repositories from its own filesystem.
- Symlinks: `TestCacheRefusesSymlinksOutOfTheCheckout` accepts an in-checkout symlink and refuses absolute and `../` targets; disabling the check makes it fail (mutation checked).
- Tests: fixtures now use go-git with an in-process file server, so neither tests nor the controller need a git binary; all cache tests pass. Kind e2e (7/7) cloned the GitHub fixture repository over HTTPS with the git-less image.
