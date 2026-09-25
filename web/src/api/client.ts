import { useCallback, useEffect, useRef, useState } from "react";
import type { APIErrorBody } from "./types";

/** An API call that failed with a non-2xx status. */
export class APIError extends Error {
  readonly status: number;
  readonly needNamespace: boolean;

  constructor(status: number, message: string, needNamespace = false) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.needNamespace = needNamespace;
  }
}

/**
 * GETs a JSON API path. A 401 means the session is gone or expired, so the
 * browser goes back to the login page instead of rendering a broken view.
 */
export async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });
  if (res.status === 401) {
    location.href = "/login";
    throw new APIError(401, "unauthorized");
  }
  if (!res.ok) {
    let body: Partial<APIErrorBody> = {};
    try {
      body = (await res.json()) as APIErrorBody;
    } catch {
      // A non-JSON error body carries nothing more than the status.
    }
    throw new APIError(
      res.status,
      body.error ?? res.statusText ?? "request failed",
      body.needNamespace === true,
    );
  }
  return (await res.json()) as T;
}

export interface Poll<T> {
  data: T | undefined;
  error: APIError | Error | undefined;
  refreshedAt: Date | undefined;
  reload: () => void;
}

/** The console's refresh interval. */
export const REFRESH_MS = 10000;

/**
 * Fetches path now and every intervalMs. The last good data stays visible
 * while a later refresh fails, so a slow API server shows a banner rather
 * than an empty page. A null path fetches nothing.
 */
export function usePoll<T>(
  path: string | null,
  intervalMs = REFRESH_MS,
): Poll<T> {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<APIError | Error>();
  const [refreshedAt, setRefreshedAt] = useState<Date>();
  const seq = useRef(0);

  const load = useCallback(() => {
    if (path == null) return;
    const mine = ++seq.current;
    getJSON<T>(path).then(
      (d) => {
        if (mine !== seq.current) return;
        setData(d);
        setError(undefined);
        setRefreshedAt(new Date());
      },
      (e: unknown) => {
        if (mine !== seq.current) return;
        setError(e instanceof Error ? e : new Error(String(e)));
      },
    );
  }, [path]);

  useEffect(() => {
    setData(undefined);
    setError(undefined);
    if (path == null) return;
    load();
    const t = setInterval(load, intervalMs);
    return () => {
      clearInterval(t);
      seq.current++;
    };
  }, [path, intervalMs, load]);

  return { data, error, refreshedAt, reload: load };
}
