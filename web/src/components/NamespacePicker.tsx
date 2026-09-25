import { useState, type FormEvent } from "react";
import { usePoll, APIError } from "../api/client";
import { Alert, Button, Input, Select } from "../ui/azrty";

/**
 * Shown when a cluster-wide list is forbidden. It offers the namespaces the
 * user may list, or a text field when even that is forbidden; the shell
 * remembers the choice in localStorage ksync.namespace.
 */
export function NamespacePicker({ onPick }: { onPick: (ns: string) => void }) {
  const namespaces = usePoll<string[]>("/api/namespaces");
  const [typed, setTyped] = useState("");
  const forbidden =
    namespaces.error instanceof APIError && namespaces.error.status === 403;
  const options = namespaces.data ?? [];
  const [chosen, setChosen] = useState("");
  const typedEntry =
    forbidden || (namespaces.data !== undefined && options.length === 0);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const ns = (typedEntry ? typed : chosen || options[0] || "").trim();
    if (ns) onPick(ns);
  };
  return (
    <form className="ks-nspick" onSubmit={submit}>
      <Alert tone="info" title="Choose a namespace">
        Your permissions do not allow listing across the whole cluster. Pick a
        namespace you can read.
      </Alert>
      <div className="ks-nspick__row">
        {typedEntry ? (
          <Input
            label="Namespace"
            mono
            placeholder="team-a"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
          />
        ) : (
          <Select
            label="Namespace"
            options={options}
            value={chosen || options[0] || ""}
            onChange={(e) => setChosen(e.target.value)}
          />
        )}
        <Button type="submit" variant="secondary" icon="arrow-right">
          Show
        </Button>
      </div>
    </form>
  );
}
