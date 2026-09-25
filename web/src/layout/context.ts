import { createContext, useContext, useEffect } from "react";
import type { Me } from "../api/types";

export interface ShellState {
  me: Me | undefined;
  /** The namespace every list is scoped to; "" lists cluster-wide. */
  namespace: string;
  setNamespace: (ns: string) => void;
  /** Pages report their last successful refresh for the top bar. */
  reportRefresh: (at: Date | undefined) => void;
  /** Pages set the last breadcrumb, such as an Application's name. */
  setDetailCrumb: (crumb: string | undefined) => void;
}

export const ShellContext = createContext<ShellState>({
  me: undefined,
  namespace: "",
  setNamespace: () => undefined,
  reportRefresh: () => undefined,
  setDetailCrumb: () => undefined,
});

export function useShell(): ShellState {
  return useContext(ShellContext);
}

/** Reports a poll's refresh time to the top bar. */
export function useReportRefresh(at: Date | undefined): void {
  const { reportRefresh } = useShell();
  useEffect(() => reportRefresh(at), [at, reportRefresh]);
}

/** Appends ?namespace= to a list path when the shell is scoped to one. */
export function withNamespace(path: string, ns: string): string {
  return ns ? path + "?namespace=" + encodeURIComponent(ns) : path;
}
