import type { ReactNode } from "react";
import { APIError, type Poll } from "../api/client";
import { NamespacePicker } from "../components/NamespacePicker";
import { useReportRefresh, useShell } from "../layout/context";
import { Alert } from "../ui/azrty";
import "./pages.css";

/** A page's eyebrow, heading and lead copy. */
export function PageHead({
  eyebrow,
  title,
  lead,
}: {
  eyebrow: string;
  title: string;
  lead: ReactNode;
}) {
  return (
    <div className="ks-head">
      <span className="az-eyebrow ks-head__eyebrow">{eyebrow}</span>
      <h1 className="ks-head__title">{title}</h1>
      <p className="ks-head__lead">{lead}</p>
    </div>
  );
}

/**
 * Reports a list poll's refresh time and renders what its failure needs:
 * the namespace picker for a forbidden cluster-wide list, or a banner that
 * keeps the last good data visible.
 */
export function PollState<T>({ poll }: { poll: Poll<T> }) {
  const { setNamespace } = useShell();
  useReportRefresh(poll.refreshedAt);
  const err = poll.error;
  if (!err) return null;
  if (err instanceof APIError && err.needNamespace)
    return <NamespacePicker onPick={setNamespace} />;
  if (err instanceof APIError && err.status === 403) {
    return (
      <Alert tone="warn" title="Not visible with your permissions">
        Your Kubernetes RBAC does not allow reading these objects here.
      </Alert>
    );
  }
  return (
    <Alert tone="warn" title="Refresh failed">
      {err instanceof APIError && err.status === 504
        ? "The API server did not answer in time. Showing the last data received."
        : "The cluster could not be read. Showing the last data received."}
    </Alert>
  );
}
