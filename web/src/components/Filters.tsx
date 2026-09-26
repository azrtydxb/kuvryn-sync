import { useCallback, useMemo, type ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { readFilters, writeFilters, type Filters } from "../filters";
import {
  Button,
  EmptyState,
  Input,
  SegmentedControl,
  Select,
} from "../ui/azrty";
import "./filters.css";

export interface FilterState {
  filters: Filters;
  /** Sets some filters, keeping the others; replace skips a history entry. */
  set: (patch: Filters, opts?: { replace?: boolean }) => void;
  clear: () => void;
}

/** The named filters, read from and written to the URL's query string. */
export function useFilters(keys: readonly string[]): FilterState {
  const [params, setParams] = useSearchParams();
  const filters = useMemo(() => readFilters(params, keys), [params, keys]);
  const set = useCallback(
    (patch: Filters, opts?: { replace?: boolean }) =>
      setParams(
        (prev) =>
          writeFilters(prev, keys, { ...readFilters(prev, keys), ...patch }),
        { replace: opts?.replace },
      ),
    [keys, setParams],
  );
  const clear = useCallback(
    () => setParams((prev) => writeFilters(prev, keys, {})),
    [keys, setParams],
  );
  return { filters, set, clear };
}

/** A table's filter controls with its "N of M" count. */
export function FilterBar({
  label,
  shown,
  total,
  filtered,
  onClear,
  children,
}: {
  label: string;
  shown: number;
  total: number;
  filtered: boolean;
  onClear: () => void;
  children: ReactNode;
}) {
  return (
    <div className="ks-filters" role="search" aria-label={label}>
      {children}
      <div className="ks-filters__end">
        {filtered && (
          <Button size="sm" variant="ghost" icon="x" onClick={onClear}>
            Clear filters
          </Button>
        )}
        <span className="ks-filters__count" aria-live="polite">
          {shown} of {total}
        </span>
      </div>
    </div>
  );
}

/** A search box; typing replaces the URL rather than adding history. */
export function SearchFilter({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string | undefined;
  onChange: (q: string) => void;
}) {
  return (
    <Input
      className="ks-filters__search"
      type="search"
      size="sm"
      icon="search"
      aria-label={label}
      placeholder={label + "…"}
      value={value ?? ""}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}

/** A dropdown of "All" plus the given values. */
export function SelectFilter({
  label,
  all,
  value,
  options,
  onChange,
}: {
  label: string;
  /** The "All" option's text, such as "All sync states". */
  all: string;
  value: string | undefined;
  options: (string | { value: string; label: string })[];
  onChange: (v: string) => void;
}) {
  return (
    <Select
      className="ks-filters__select"
      id={"ks-filter-" + label.toLowerCase().replace(/[^a-z0-9]+/g, "-")}
      label={label}
      size="sm"
      value={value ?? ""}
      options={[{ value: "", label: all }, ...options]}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}

/** A segmented "All" plus the given values, for short lists of states. */
export function SegmentFilter({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string | undefined;
  options: string[];
  onChange: (v: string) => void;
}) {
  return (
    <div className="ks-filters__field">
      <span className="ks-filters__label" aria-hidden="true">
        {label}
      </span>
      <SegmentedControl
        aria-label={label}
        value={value ?? ""}
        options={[
          { value: "", label: "All" },
          ...options.map((o) => ({ value: o, label: o })),
        ]}
        onChange={onChange}
      />
    </div>
  );
}

/** An on/off filter in the design system's switch style. */
export function SwitchFilter({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (on: boolean) => void;
}) {
  return (
    <label className="az-check ks-filters__switch">
      <input
        type="checkbox"
        className="az-check__input"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="az-switch__track" aria-hidden="true" />
      {label}
    </label>
  );
}

/** What a filtered table shows when no row matches. */
export function NoMatch({
  what,
  onClear,
}: {
  what: string;
  onClear: () => void;
}) {
  return (
    <EmptyState
      icon="filter-x"
      title={"No " + what + " match these filters"}
      description="Change or clear the filters to see more."
      action={
        <Button size="sm" variant="secondary" icon="x" onClick={onClear}>
          Clear filters
        </Button>
      }
    />
  );
}
