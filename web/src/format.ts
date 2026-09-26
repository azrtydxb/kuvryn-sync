// Display helpers. Unknown values stay "—", as the API sends them.

export const DASH = "—";

/** A time as "just now", "4 min ago", "3 h ago" or "2 d ago". */
export function ago(iso: string | undefined, now = Date.now()): string {
  if (!iso || iso === DASH) return DASH;
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return DASH;
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 60) return "just now";
  if (s < 3600) return Math.floor(s / 60) + " min ago";
  if (s < 86400) return Math.floor(s / 3600) + " h ago";
  return Math.floor(s / 86400) + " d ago";
}

/** A Git commit shortened to 7 characters; anything else unchanged. */
export function shortSha(sha: string): string {
  return /^[0-9a-f]{40}$/.test(sha) ? sha.slice(0, 7) : sha;
}

/** A digest shortened to its algorithm, first and last characters. */
export function shortDigest(digest: string): string {
  const m = /^([a-z0-9]+):([0-9a-f]+)$/.exec(digest);
  if (!m || m[2].length <= 12) return digest;
  return m[1] + ":" + m[2].slice(0, 4) + "…" + m[2].slice(-4);
}

/** A plan summary as "+1 ~2 −0". */
export function planSummary(p: {
  create: number;
  update: number;
  delete: number;
}): string {
  return "+" + p.create + " ~" + p.update + " −" + p.delete;
}

/** hh:mm:ss in 24-hour time. */
export function clock(d: Date): string {
  return d.toLocaleTimeString("en-GB", { hour12: false });
}

/**
 * A time as an absolute local timestamp, "2026-09-26 14:03:12 GMT+4", for
 * the tooltips on relative times.
 */
export function absolute(iso: string | undefined): string {
  if (!iso || iso === DASH) return DASH;
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return DASH;
  const d = new Date(t);
  const p = (n: number) => String(n).padStart(2, "0");
  const zone = new Intl.DateTimeFormat("en-GB", { timeZoneName: "short" })
    .formatToParts(d)
    .find((part) => part.type === "timeZoneName")?.value;
  return (
    d.getFullYear() +
    "-" +
    p(d.getMonth() + 1) +
    "-" +
    p(d.getDate()) +
    " " +
    p(d.getHours()) +
    ":" +
    p(d.getMinutes()) +
    ":" +
    p(d.getSeconds()) +
    (zone ? " " + zone : "")
  );
}
