export type AuthContextType = {
  user: string | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshAuth: () => Promise<void>;
};

export type QueryRow = {
  timestamp: string;
  domain: string;
  qtype: number;
  client_ip: string;
  client_label?: string;
  blocked: boolean;
  category?: string;
  confidence?: number;
  reason?: string;
};

export type RuleScopeType = "global" | "group" | "client";

export type RuleRow = {
  id: number;
  scope_type: RuleScopeType;
  scope_key: string;
  label: string;
  blocked: boolean;
  rules: string;
  created_at: string;
  updated_at?: string;
  display_name?: string;
};

export type GroupMember = {
  id: number;
  identifier: string;
  type: "ip" | "mac";
};

export type ClientGroup = {
  id: number;
  name: string;
  label: string;
  blocked: boolean;
  created_at: string;
  members: GroupMember[];
  rule_count?: number;
};

export type ClientRecord = {
  id: number;
  scope_type: RuleScopeType;
  scope_key: string;
  client_ip: string;
  label: string;
  blocked: boolean;
  rules: string;
  created_at: string;
  display_name?: string;
};

export type ServiceDef = {
  id: string;
  name: string;
  icon: string;
  category: string;
  domains: string[];
};

export type ServiceSchedule = {
  enabled: boolean;
  days_of_week: string;
  time_start: string;
  time_end: string;
  /** "client" = this client has an explicit override; "global" = inherited from global schedule */
  source?: "client" | "global";
};

export type ServiceScheduleMap = Record<string, ServiceSchedule>;

export type MLSettings = {
  threshold: number;
  block_dga: boolean;
  block_phishing: boolean;
  block_malware: boolean;
  block_other: boolean;
  cache_size: number;
  cache_max: number;
  cache_ttl_min: number;
  feedback_total: number;
  feedback_safe: number;
  feedback_mal: number;
  ml_connected: boolean;
};

export type ReasonColors = { bg: string; fg: string; border: string };

export type DNSInstructionStep = { text?: string; code?: string };

export type DNSInstructionSet = {
  label: string;
  steps: DNSInstructionStep[];
  note?: string;
};
