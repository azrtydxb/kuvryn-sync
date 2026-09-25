import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Alert, Button, Logo, ProductLogo } from "../ui/azrty";
import type { Me } from "../api/types";
import { PRODUCT } from "../brand";
import { useTheme } from "../theme";
import "./login.css";

const SOCIAL: Record<string, { label: string; icon: string }> = {
  github: { label: "GitHub", icon: "github" },
  gitlab: { label: "GitLab", icon: "gitlab" },
};

// Why a sign-in failed, keyed by the reason the callback redirects with.
const FAILURES: Record<string, string> = {
  unavailable:
    "The identity provider is not reachable yet. Try again in a moment.",
  expired:
    "The sign-in took too long or was started in another browser. Try again.",
  denied: "The identity provider denied the sign-in.",
  config:
    "The identity provider rejected the console's sign-in request. Ask your cluster admin to check its OIDC client.",
  claims:
    "Your identity provider did not send the claims this console needs, or sent a system: identity. Ask your cluster admin.",
  groups:
    "Your account belongs to too many groups for a session. Ask your cluster admin.",
};

/** The login page from "Kuvryn Sync Login.dc.html". */
export default function Login() {
  useTheme();
  const [params] = useSearchParams();
  const [me, setMe] = useState<Me>();
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    // /api/me answers without a session, with the cluster and connectors.
    fetch("/api/me", { credentials: "same-origin" })
      .then((r) => (r.ok ? (r.json() as Promise<Me>) : undefined))
      .then(setMe, () => undefined);
  }, []);

  const connectors = me?.connectors ?? [];
  const social = connectors.filter((c) => c in SOCIAL);
  const error = params.get("error");
  const signIn = (connector?: string) => {
    setLoading(true);
    location.href =
      "/auth/start" +
      (connector ? "?connector=" + encodeURIComponent(connector) : "");
  };
  const ssoName = me?.ssoName;

  return (
    <div className="ks-login">
      <main className="ks-login__main">
        <div className="ks-login__brand">
          <ProductLogo
            {...PRODUCT}
            layout="icon"
            size={44}
            className="ks-login__logo"
          />
          <span className="ks-login__word">
            <span>Kuvryn</span>
            <span className="ks-login__word-sub">Sync</span>
          </span>
        </div>
        <div className="ks-login__form">
          <div className="ks-login__intro">
            <span className="ks-login__eyebrow">GitOps for Kubernetes</span>
            <h1 className="ks-login__title">Welcome back</h1>
            <p className="ks-login__lead">
              Sign in to view applications, plans and revisions. What you can
              see follows your Kubernetes RBAC.
            </p>
          </div>
          {error != null && (
            <Alert tone="bad" title="Sign-in failed">
              {FAILURES[error] ??
                "Try again, or ask your cluster admin for access."}
            </Alert>
          )}
          <Button
            size="lg"
            block
            icon="key-round"
            onClick={() => signIn()}
            disabled={loading}
          >
            {loading
              ? "Signing in…"
              : ssoName
                ? "Sign in with " + ssoName
                : "Single sign-on"}
          </Button>
          {social.length > 0 && (
            <div className="ks-login__social">
              {social.map((c) => (
                <Button
                  key={c}
                  variant="secondary"
                  block
                  icon={SOCIAL[c].icon}
                  onClick={() => signIn(c)}
                  disabled={loading}
                >
                  {SOCIAL[c].label}
                </Button>
              ))}
            </div>
          )}
          {connectors.includes("local") && (
            <>
              <div className="ks-login__divider">or with email</div>
              <Button
                variant="secondary"
                size="lg"
                block
                iconRight="arrow-right"
                onClick={() => signIn("local")}
                disabled={loading}
              >
                Sign in with email
              </Button>
            </>
          )}
          <p className="ks-login__note">
            Read-only access. Changes go through Git or the CLI. Ask your
            cluster admin for access.
          </p>
        </div>
        <div className="ks-login__foot">
          <span className="ks-login__maker">
            An Azrty product
            <Logo variant="mark" size={18} />
          </span>
          <span className="ks-login__links">
            {me?.docsURL && <a href={me.docsURL}>Docs</a>}
            {me?.statusURL && <a href={me.statusURL}>Status</a>}
            <span className="ks-login__cluster">
              {(me?.cluster ?? "—") + " · v1alpha1"}
            </span>
          </span>
        </div>
      </main>
      <aside className="ks-login__panel" aria-label="Kuvryn Sync">
        <div className="ks-login__rings">
          <span className="ks-login__ring ks-login__ring--1" />
          <span className="ks-login__ring ks-login__ring--2" />
          <span className="ks-login__ring ks-login__ring--3" />
          <ProductLogo
            {...PRODUCT}
            layout="icon"
            size={210}
            className="ks-login__panel-logo"
          />
        </div>
        <div className="ks-login__panel-text">
          <h2 className="ks-login__panel-title">GitOps that sticks.</h2>
          <p className="ks-login__panel-lead">
            Every commit planned, applied and explained.
          </p>
        </div>
      </aside>
    </div>
  );
}
