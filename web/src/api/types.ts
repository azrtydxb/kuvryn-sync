// Mirrors the view models in internal/console/views.go and api.go.

export interface Me {
  authenticated: boolean;
  username?: string;
  groups?: string[];
  /** How the session signed in; set only when authenticated. */
  method?: "token" | "oidc";
  cluster: string;
  /** Token sign-in is always offered. */
  tokenSignIn: boolean;
  /** Whether OIDC sign-in is configured; connectors are empty without it. */
  oidc: boolean;
  connectors: string[];
  ssoName?: string;
  docsURL?: string;
  statusURL?: string;
}

export interface AppRow {
  name: string;
  namespace: string;
  destination: string;
  repository: string;
  path: string;
  render: string;
  commit: string;
  sync: string;
  health: string;
  lastReconcile: string;
}

export interface SourceView {
  repository: string;
  revision: string;
  path: string;
  render: string;
}

export interface PolicyView {
  automatic: boolean;
  prune: boolean;
  selfHeal: boolean;
  suspend: boolean;
  conflictPolicy: string;
  failureAction: string;
  deletionPolicy: string;
  serviceAccountName: string;
}

export interface ConditionView {
  type: string;
  status: string;
  reason: string;
  message: string;
  lastTransitionTime: string;
}

export interface ChainLink {
  kind: string;
  name: string;
  state: string;
}

export interface Cause {
  resource: string;
  reason: string;
  message: string;
  chain: ChainLink[];
}

export interface RefView {
  apiVersion: string;
  kind: string;
  namespace?: string;
  name: string;
}

export interface ChangeView {
  path: string;
  before: string;
  after: string;
  redacted: boolean;
}

export interface PlanResourceView {
  action: string;
  ref: RefView;
  changes: ChangeView[];
  warnings: string[];
}

export interface SummaryView {
  create: number;
  update: number;
  delete: number;
  unchanged: number;
}

export interface PlanView {
  revision: string;
  commit: string;
  digest: string;
  phase: string;
  summary: SummaryView;
  truncated: boolean;
  resources: PlanResourceView[];
}

export interface AppDetail extends AppRow {
  state: string;
  desiredRevision: string;
  deployedRevision: string;
  source: SourceView;
  policy: PolicyView;
  conditions: ConditionView[];
  diagnosis: Cause[];
  plan: PlanView | null;
  planVisible: boolean;
}

export interface RevisionRow {
  name: string;
  namespace: string;
  application: string;
  commit: string;
  phase: string;
  plan: SummaryView;
  digest: string;
  approvedBy: string;
  attempts: number;
  started: string;
  failure: string;
}

export interface ResourceRow {
  kind: string;
  name: string;
  apiVersion: string;
  sync: string;
  health: string;
  visible: boolean;
}

export interface RepoRow {
  name: string;
  namespace: string;
  url: string;
  ref: string;
  observed: string;
  state: string;
  message: string;
  apps: number | null;
  poll: string;
  webhook: boolean;
  lastFetch: string;
}

export interface ImagePolicyRow {
  name: string;
  namespace: string;
  image: string;
  rule: string;
  latest: string;
  digest: string;
  lastScan: string;
}

export interface APIErrorBody {
  error: string;
  needNamespace?: boolean;
}
