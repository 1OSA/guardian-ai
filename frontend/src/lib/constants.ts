/** Accent colour — still needed for recharts, icon tints, and dynamic border colours. */
export const ACCENT = "#798777";

/** Polling intervals (ms) — change here to affect the whole app. */
export const POLL_DASHBOARD_MS = 10_000;
export const POLL_QUERIES_LIVE_MS = 3_000;
export const POLL_RELATIVE_TIME_MS = 15_000;

export const PAGE_SIZE = 50;

export const DAY_LABELS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

export const QTYPE_MAP: Record<number, string> = {
  1: "A",
  2: "NS",
  5: "CNAME",
  6: "SOA",
  12: "PTR",
  15: "MX",
  16: "TXT",
  28: "AAAA",
  33: "SRV",
  255: "ANY",
};

export const SIDEBAR_W = 220;
