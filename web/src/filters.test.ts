import { describe, expect, it } from "vitest";
import type {
  AppRow,
  ImagePolicyRow,
  RepoRow,
  ResourceRow,
  RevisionRow,
} from "./api/types";
import {
  APP_FILTERS,
  filterApps,
  filterHistory,
  filterPolicies,
  filterRepos,
  filterResources,
  filterRevisions,
  isFiltered,
  matchesQuery,
  NOT_SYNCED,
  presentValues,
  readFilters,
  RESOURCE_FILTERS,
  writeFilters,
} from "./filters";

function app(
  name: string,
  sync: string,
  health: string,
  more: Partial<AppRow> = {},
): AppRow {
  return {
    name,
    namespace: "default",
    destination: name,
    repository: "platform",
    path: "apps/" + name,
    render: "helm",
    commit: "—",
    sync,
    health,
    lastReconcile: "—",
    lastChange: "—",
    ...more,
  };
}

const APPS = [
  app("payments", "OutOfSync", "Degraded"),
  app("checkout", "AwaitingApproval", "Healthy"),
  app("catalog", "Synced", "Healthy"),
  app("search", "Drifted", "Healthy", { path: "apps/scout" }),
  app("ledger", "Unknown", "Suspended", { repository: "finance-gitops" }),
];

const names = (rows: { name: string }[]) => rows.map((r) => r.name);

describe("TestFiltersRoundTripThroughTheURL", () => {
  it("reads only the named, non-empty filters", () => {
    const params = new URLSearchParams(
      "q=scout&sync=OutOfSync&health=&other=x",
    );
    expect(readFilters(params, APP_FILTERS)).toEqual({
      q: "scout",
      sync: "OutOfSync",
    });
  });

  it("writes filters, keeps other parameters and drops empty ones", () => {
    const next = writeFilters(
      new URLSearchParams("namespace=shop&health=Degraded&q=old"),
      APP_FILTERS,
      { q: "a b/c&d", sync: "OutOfSync", health: "" },
    );
    expect(next.get("namespace")).toBe("shop");
    expect(next.has("health")).toBe(false);
    expect(next.toString()).toBe("namespace=shop&q=a+b%2Fc%26d&sync=OutOfSync");
  });

  it("round-trips through a query string", () => {
    const f = { q: "scout", sync: "OutOfSync", health: "Degraded" };
    const search = "?" + writeFilters(new URLSearchParams(), APP_FILTERS, f);
    expect(search).toBe("?q=scout&sync=OutOfSync&health=Degraded");
    expect(readFilters(new URLSearchParams(search), APP_FILTERS)).toEqual(f);
  });

  it("clears every filter it names and nothing else", () => {
    const next = writeFilters(
      new URLSearchParams("q=x&kind=Pod&attention=1&tab=keep"),
      RESOURCE_FILTERS,
      {},
    );
    expect(next.toString()).toBe("tab=keep");
    expect(isFiltered(readFilters(next, RESOURCE_FILTERS))).toBe(false);
    expect(isFiltered({ q: "x" })).toBe(true);
  });
});

describe("TestFilterMatching", () => {
  it("searches case-insensitively, every term somewhere", () => {
    expect(matchesQuery("", "anything")).toBe(true);
    expect(matchesQuery("PAY", "payments")).toBe(true);
    expect(matchesQuery("apps pay", "payments", "apps/payments")).toBe(true);
    expect(matchesQuery("apps nope", "payments", "apps/payments")).toBe(false);
  });

  it("filters Applications by name, path and repository", () => {
    expect(names(filterApps(APPS, { q: "scout" }))).toEqual(["search"]);
    expect(names(filterApps(APPS, { q: "finance" }))).toEqual(["ledger"]);
    expect(names(filterApps(APPS, { q: "check" }))).toEqual(["checkout"]);
  });

  it("filters Applications by sync and health together", () => {
    expect(names(filterApps(APPS, { health: "Healthy" }))).toEqual([
      "checkout",
      "catalog",
      "search",
    ]);
    expect(
      names(filterApps(APPS, { health: "Healthy", sync: "Drifted" })),
    ).toEqual(["search"]);
    expect(names(filterApps(APPS, { sync: "AwaitingApproval" }))).toEqual([
      "checkout",
    ]);
  });

  it("treats Not synced as every converging or diverged state", () => {
    expect(names(filterApps(APPS, { sync: NOT_SYNCED }))).toEqual([
      "payments",
      "search",
    ]);
  });

  it("lists the states present in a stable order, keeping the selected one", () => {
    expect(
      presentValues(
        ["Healthy", "Degraded", "Healthy", "Zeta"],
        ["Degraded", "Healthy"],
      ),
    ).toEqual(["Degraded", "Healthy", "Zeta"]);
    expect(presentValues(["Healthy"], ["Healthy"], "Degraded")).toEqual([
      "Healthy",
      "Degraded",
    ]);
    expect(presentValues(["—", "Pod"], [])).toEqual(["Pod"]);
  });

  it("filters Revisions, history, Repositories and Image policies", () => {
    const rev = (name: string, application: string, phase: string) =>
      ({
        name,
        application,
        phase,
        commit: "abc",
        approvedBy: "—",
      }) as RevisionRow;
    const revs = [
      rev("payments-1", "payments", "Failed"),
      rev("catalog-1", "catalog", "Healthy"),
      rev("catalog-2", "catalog", "Failed"),
    ];
    expect(
      names(filterRevisions(revs, { app: "catalog", phase: "Failed" })),
    ).toEqual(["catalog-2"]);
    expect(names(filterRevisions(revs, { q: "pay" }))).toEqual(["payments-1"]);
    expect(names(filterHistory(revs, { phase: "Healthy" }))).toEqual([
      "catalog-1",
    ]);

    const repo = (name: string, url: string, state: string) =>
      ({ name, url, state, ref: "main", message: "—" }) as RepoRow;
    const repos = [
      repo("platform", "https://github.com/acme/platform.git", "Ready"),
      repo("finance-gitops", "git@gitlab.acme.io:finance/gitops.git", "Failed"),
    ];
    expect(names(filterRepos(repos, { state: "Failed" }))).toEqual([
      "finance-gitops",
    ]);
    expect(names(filterRepos(repos, { q: "github" }))).toEqual(["platform"]);

    const policies = [
      {
        name: "search",
        image: "ghcr.io/acme/search",
        rule: "semver ^0.12",
        latest: "0.12.3",
      },
      {
        name: "checkout",
        image: "ghcr.io/acme/checkout",
        rule: "semver ~2.4",
        latest: "2.4.7",
      },
    ] as ImagePolicyRow[];
    expect(names(filterPolicies(policies, { q: "acme/check" }))).toEqual([
      "checkout",
    ]);
  });

  it("filters resources by kind and to those needing attention", () => {
    const res = (kind: string, name: string, sync: string, health: string) =>
      ({
        kind,
        name,
        apiVersion: "v1",
        sync,
        health,
        visible: sync !== "—",
      }) as ResourceRow;
    const rows = [
      res("Deployment", "api", "OutOfSync", "Degraded"),
      res("ConfigMap", "api-config", "OutOfSync", "—"),
      res("Service", "api", "Synced", "Healthy"),
      res("Service", "api-canary", "Synced", "Progressing"),
      res("Secret", "db", "—", "—"),
    ];
    expect(names(filterResources(rows, { kind: "Service" }))).toEqual([
      "api",
      "api-canary",
    ]);
    expect(names(filterResources(rows, { attention: "1" }))).toEqual([
      "api",
      "api-config",
      "api-canary",
    ]);
    expect(
      names(filterResources(rows, { q: "canary", attention: "1" })),
    ).toEqual(["api-canary"]);
  });
});
