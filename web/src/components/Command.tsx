import { useEffect, useState } from "react";
import { IconButton } from "../ui/azrty";
import { Tip } from "./Tip";

/**
 * The design system's CodeBlock markup (az-code), with its copy button's
 * label in a keyboard-reachable tooltip instead of a native title.
 */
export function Command({ title, code }: { title: string; code: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => setCopied(false), 1400);
    return () => clearTimeout(t);
  }, [copied]);
  const copy = () => {
    navigator.clipboard?.writeText(code).catch(() => undefined);
    setCopied(true);
  };
  const label = copied ? "Copied" : "Copy command";
  return (
    <div className="az-code">
      <div className="az-code__head">
        <span>{title}</span>
        <Tip text={label} asChild side="bottom" align="end">
          <IconButton
            icon={copied ? "check" : "copy"}
            label={label}
            size={14}
            onClick={copy}
          />
        </Tip>
      </div>
      <pre>{code}</pre>
    </div>
  );
}
