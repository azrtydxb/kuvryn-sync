import { useCallback, useEffect, useMemo, useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { getJSON, usePoll } from "../api/client";
import type { AppRow, Me } from "../api/types";
import { PRODUCT } from "../brand";
import { Tip } from "../components/Tip";
import { clock, DASH } from "../format";
import { useTheme } from "../theme";
import { LIVE_TIP, READ_ONLY_TIP } from "../tips";
import {
  Badge,
  Button,
  Icon,
  IconButton,
  ProductLogo,
  Topbar,
} from "../ui/azrty";
import { ShellContext, withNamespace, type ShellState } from "./context";
import "./shell.css";

const NS_KEY = "ksync.namespace";

const NAV = [
  { id: "apps", path: "/apps", label: "Applications", icon: "boxes" },
  {
    id: "repos",
    path: "/repositories",
    label: "Repositories",
    icon: "git-branch",
  },
  { id: "revs", path: "/revisions", label: "Revisions", icon: "history" },
  {
    id: "images",
    path: "/imagepolicies",
    label: "Image policies",
    icon: "container",
  },
];

function storedNamespace(): string {
  try {
    return localStorage.getItem(NS_KEY) ?? "";
  } catch {
    return "";
  }
}

/** The console frame from "Kuvryn Sync Console.dc.html": sidebar and top bar. */
export default function Shell() {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const [theme, setTheme] = useTheme();
  const [me, setMe] = useState<Me>();
  const [namespace, setNamespaceState] = useState(storedNamespace);
  const [refreshedAt, setRefreshedAt] = useState<Date>();
  const [detailCrumb, setDetailCrumb] = useState<string>();

  useEffect(() => {
    getJSON<Me>("/api/me").then(
      (m) => {
        if (!m.authenticated) navigate("/login", { replace: true });
        else setMe(m);
      },
      () => undefined,
    );
  }, [navigate]);

  const setNamespace = useCallback((ns: string) => {
    try {
      if (ns) localStorage.setItem(NS_KEY, ns);
      else localStorage.removeItem(NS_KEY);
    } catch {
      // Without storage the choice lasts for this page.
    }
    setNamespaceState(ns);
  }, []);

  // The Applications badge counts Degraded Applications.
  const apps = usePoll<AppRow[]>(
    me ? withNamespace("/api/applications", namespace) : null,
  );
  const degraded = (apps.data ?? []).filter(
    (a) => a.health === "Degraded",
  ).length;

  const themeLabel =
    theme === "dark" ? "Use the light theme" : "Use the dark theme";
  const active = NAV.find((n) => pathname.startsWith(n.path)) ?? NAV[0];
  const cluster = me?.cluster ?? DASH;
  const crumbs = [cluster, active.label].concat(
    detailCrumb ? [detailCrumb] : [],
  );
  const state: ShellState = useMemo(
    () => ({
      me,
      namespace,
      setNamespace,
      reportRefresh: setRefreshedAt,
      setDetailCrumb,
    }),
    [me, namespace, setNamespace],
  );

  return (
    <ShellContext.Provider value={state}>
      <div className="ks-shell">
        <aside className="az-sidebar">
          <div className="az-sidebar__brand">
            <ProductLogo {...PRODUCT} layout="horizontal" size={48} />
          </div>
          <div className="az-sidebar__workspace">
            <span className="az-eyebrow">Cluster</span>
            <div className="ks-shell__cluster">
              <Icon
                name="server"
                size={14}
                className="ks-shell__cluster-icon"
              />
              {cluster}
            </div>
            {namespace && (
              <div className="ks-shell__ns">
                <span>
                  Namespace <b>{namespace}</b>
                </span>
                <button
                  type="button"
                  className="ks-shell__ns-clear"
                  onClick={() => setNamespace("")}
                >
                  All
                </button>
              </div>
            )}
          </div>
          <nav className="az-nav" aria-label="Console">
            {NAV.map((n) => (
              <button
                key={n.id}
                type="button"
                className={
                  "az-nav__item" +
                  (n.id === active.id ? " az-nav__item--active" : "")
                }
                aria-current={n.id === active.id ? "page" : undefined}
                onClick={() => navigate(n.path)}
              >
                <Icon name={n.icon} size={17} />
                <span>{n.label}</span>
                {n.id === "apps" && degraded > 0 && (
                  <span className="az-nav__badge">{degraded}</span>
                )}
              </button>
            ))}
          </nav>
          <div className="az-sidebar__foot">
            <div className="az-connected">
              <span className="az-dot az-dot--pulse" />
              Watching sync.kuvryn.io/v1alpha1
            </div>
            <div className="ks-shell__rbac">
              <Icon name="eye" size={14} />
              Read-only · viewer RBAC
            </div>
            {me?.username && (
              <form method="post" action="/logout" className="ks-shell__user">
                <Tip
                  text={me.username}
                  align="start"
                  className="ks-shell__usertip"
                >
                  <span className="ks-shell__username">{me.username}</span>
                </Tip>
                <Button type="submit" variant="ghost" size="sm" icon="log-out">
                  Sign out
                </Button>
              </form>
            )}
          </div>
        </aside>
        <div className="ks-shell__body">
          {/* LIVE is drawn here rather than by Topbar, to carry a tooltip. */}
          <Topbar crumbs={crumbs} live={false}>
            <Tip text={LIVE_TIP} side="bottom">
              <span className="az-live">
                <span className="az-dot az-dot--pulse" />
                LIVE
              </span>
            </Tip>
            <span className="ks-shell__refreshed">
              {refreshedAt ? "Refreshed " + clock(refreshedAt) : "Loading"}
            </span>
            <Tip text={READ_ONLY_TIP} side="bottom" align="end">
              <Badge tone="pillar" icon="eye">
                Read-only
              </Badge>
            </Tip>
            <Tip text={themeLabel} side="bottom" align="end" asChild>
              <IconButton
                icon={theme === "dark" ? "sun" : "moon"}
                label={themeLabel}
                size={15}
                onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              />
            </Tip>
          </Topbar>
          <main className="ks-shell__main">
            <div className="ks-shell__content">{me ? <Outlet /> : null}</div>
          </main>
        </div>
      </div>
    </ShellContext.Provider>
  );
}
