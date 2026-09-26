import {
  cloneElement,
  isValidElement,
  useId,
  type ReactElement,
  type ReactNode,
} from "react";
import { absolute, ago, DASH, shortDigest, shortSha } from "../format";
import { Tooltip } from "../ui/azrty";
import "./tip.css";

export interface TipProps {
  /** The tooltip's text. */
  text: ReactNode;
  side?: "top" | "bottom";
  /** Anchors the bubble to the trigger's start or end instead of its middle. */
  align?: "start" | "end";
  /** Monospace and wider, for commits, digests, paths and URLs. */
  mono?: boolean;
  /**
   * The child is itself focusable (a button or link): it gets
   * aria-describedby instead of being wrapped in a focusable span.
   */
  asChild?: boolean;
  className?: string;
  children: ReactNode;
}

/**
 * The design system's CSS-only Tooltip, reachable by keyboard: the trigger
 * is focusable and described by the bubble, so the text shows on hover and
 * on focus and screen readers announce it.
 */
export function Tip({
  text,
  side,
  align,
  mono,
  asChild,
  className,
  children,
}: TipProps) {
  const id = useId();
  const classes = [
    "ks-tip",
    align && "ks-tip--" + align,
    mono && "ks-tip--mono",
    className,
  ]
    .filter(Boolean)
    .join(" ");
  const trigger =
    asChild && isValidElement(children) ? (
      cloneElement(children as ReactElement<Record<string, unknown>>, {
        "aria-describedby": id,
        // The bubble replaces a native title tooltip.
        title: undefined,
      })
    ) : (
      <span className="ks-tip__target" tabIndex={0} aria-describedby={id}>
        {children}
      </span>
    );
  return (
    <Tooltip
      content={<span id={id}>{text}</span>}
      side={side}
      className={classes}
    >
      {trigger}
    </Tooltip>
  );
}

/** A relative time with its absolute local time in a tooltip. */
export function Ago({
  iso,
  align = "end",
}: {
  iso: string;
  align?: "start" | "end";
}) {
  const rel = ago(iso);
  if (rel === DASH) return <>{DASH}</>;
  return (
    <Tip text={absolute(iso)} align={align}>
      {rel}
    </Tip>
  );
}

/** A commit shortened to 7 characters, with the full SHA in a tooltip. */
export function Sha({ sha }: { sha: string }) {
  const short = shortSha(sha);
  if (short === sha) return <>{sha}</>;
  return (
    <Tip text={sha} mono>
      {short}
    </Tip>
  );
}

/** A shortened digest, with the full digest in a tooltip. */
export function Digest({ digest }: { digest: string }) {
  const short = shortDigest(digest);
  if (short === digest) return <>{digest}</>;
  return (
    <Tip text={digest} mono>
      {short}
    </Tip>
  );
}

/**
 * A long path or URL cut to max characters with an ellipsis, with the full
 * value in a tooltip; shorter values are shown as they are.
 */
export function Clip({
  text,
  max = 36,
  align,
}: {
  text: string;
  max?: number;
  align?: "start" | "end";
}) {
  if (text.length <= max) return <>{text}</>;
  return (
    <Tip text={text} mono align={align}>
      <span className="ks-clip" style={{ maxWidth: max + "ch" }}>
        {text}
      </span>
    </Tip>
  );
}
