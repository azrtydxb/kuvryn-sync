# Migrate controllers to the events.k8s.io recorder

Status: open
Created: 2026-09-24

## Description

controller-runtime deprecates `GetEventRecorderFor` and the core
`record.EventRecorder` in favour of `GetEventRecorder` and client-go's
`tools/events` recorder. The application, repository and imagepolicy
controllers stay on the core recorder for now, because the events.k8s.io
recorder does not fit how Solder uses Events:

- It keys its series cache on type, action, reason, reporting controller and
  instance, regarding and related objects, but not on the note. A repeat within
  about 30 to 36 minutes only bumps `series.count` and keeps the first note, so
  a second `ImageSelected`, a changed `Diagnosed` cause, `ImagesUpdated` and the
  failure events would all show a stale message.
- It rejects notes over 1 KiB, and the Event is lost.
- `kubectl get events --sort-by=.lastTimestamp` no longer orders Solder's
  Events.

Migrate once each message-bearing Event can pass a distinct related object, or
once client-go keys series on the note. Done means the controllers use
`GetEventRecorder`, every Event keeps its current message, and notes are
truncated to 1 KiB.

## Acceptance criteria

- [ ] Each Event whose message changes between repeats either passes a distinct
      related object or client-go keys series on the note, shown by a test that
      records two Events with the same reason and different messages and sees
      both messages.
- [ ] Notes longer than 1 KiB are truncated rather than dropped, shown by a
      test.
- [ ] The controllers use `GetEventRecorder` and `events.EventRecorder`; the
      controller role and the Helm chart grant `create` and `patch` on
      `events.k8s.io` events, and `TestHelmChartRoleMatchesGeneratedRole`
      passes.
- [ ] `make test` and `make lint` pass.

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the task open. -->
