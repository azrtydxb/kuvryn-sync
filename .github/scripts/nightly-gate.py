#!/usr/bin/env python3
"""Decide whether a scheduled (nightly) run has anything new to test.

Writes run=true or run=false to $GITHUB_OUTPUT. It says false only when this
workflow already succeeded on main for the same commit, or for a commit that
differs only by "[skip ci]" commits (the digest pins CI writes back), and today
is not one of the forced days. Anything unexpected (API error, no earlier run,
diverged history) fails open to run=true, so a broken gate never hides a night.

Only the schedule event is gated; every other event runs.

Environment: WORKFLOW_FILE (for example heavy.yml), GH_TOKEN, and the variables
GitHub sets (GITHUB_REPOSITORY, GITHUB_SHA, GITHUB_EVENT_NAME, GITHUB_OUTPUT).
Optional: RUN_EVENT, the event of the earlier runs to compare with ("schedule"
by default; empty for any event).
"""

import datetime
import json
import os
import sys
import urllib.request

# Forced full runs, ISO weekdays (1 = Monday): base images and unpinned tools
# can change without a commit, so the nightly still runs twice a week.
FORCED_WEEKDAYS = (1, 4)


def say(run, why):
    print(f"run={run}: {why}")
    with open(os.environ["GITHUB_OUTPUT"], "a") as out:
        out.write(f"run={run}\n")


def api(path):
    req = urllib.request.Request(
        f"https://api.github.com/repos/{os.environ['GITHUB_REPOSITORY']}{path}",
        headers={
            "Authorization": f"Bearer {os.environ['GH_TOKEN']}",
            "Accept": "application/vnd.github+json",
        },
    )
    with urllib.request.urlopen(req, timeout=20) as resp:
        return json.load(resp)


def main():
    if os.environ.get("GITHUB_EVENT_NAME") != "schedule":
        return say("true", "not a scheduled run")
    weekday = datetime.datetime.now(datetime.timezone.utc).isoweekday()
    if weekday in FORCED_WEEKDAYS:
        return say("true", f"forced full run (ISO weekday {weekday})")

    sha = os.environ["GITHUB_SHA"]
    event = os.environ.get("RUN_EVENT", "schedule")
    query = "status=success&branch=main&per_page=1"
    if event:
        query += f"&event={event}"
    runs = api(f"/actions/workflows/{os.environ['WORKFLOW_FILE']}/runs?{query}")
    runs = runs.get("workflow_runs", [])
    if not runs:
        return say("true", "no earlier successful run")
    last = runs[0]["head_sha"]
    if last == sha:
        return say("false", f"already succeeded on {sha[:12]}")

    cmp = api(f"/compare/{last}...{sha}")
    commits = cmp.get("commits", [])
    if (
        cmp.get("status") in ("ahead", "identical")
        and len(commits) < 250
        and all("[skip ci]" in c["commit"]["message"] for c in commits)
    ):
        return say("false", f"only [skip ci] commits since {last[:12]}")
    return say("true", f"{len(commits)} commit(s) since the last success {last[:12]}")


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:  # noqa: BLE001 - fail open on purpose
        print(f"gate error, running anyway: {exc}", file=sys.stderr)
        say("true", "gate error")
