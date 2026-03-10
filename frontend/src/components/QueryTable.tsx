import React from "react";
import { useNavigate } from "react-router-dom";
import { FaThumbsUp, FaThumbsDown } from "react-icons/fa";
import { ACCENT, QTYPE_MAP } from "../lib/constants";
import {
  formatTime,
  relativeTime,
  reasonBadgeColor,
  reasonLabel,
  rowBg,
} from "../lib/utils";
import type { QueryRow } from "../lib/types";

export type QueryTableProps = {
  rows: QueryRow[];
  compact?: boolean;
  loading?: boolean;
  totalRows?: number;
  quickAction?: Record<string, string>;
  feedbackSent?: Record<string, string>;
  onQuickAllow?: (domain: string) => void;
  onQuickBlock?: (domain: string) => void;
  onFeedback?: (row: QueryRow, verdict: "safe" | "malicious") => void;
};

const QueryTable: React.FC<QueryTableProps> = ({
  rows,
  compact = false,
  loading = false,
  totalRows,
  quickAction = {},
  feedbackSent = {},
  onQuickAllow,
  onQuickBlock,
  onFeedback,
}) => {
  const navigate = useNavigate();
  const colSpan = compact ? 6 : 7;

  return (
    <table
      className="w-full border-collapse"
      style={{ minWidth: compact ? 560 : 640 }}
    >
      <thead>
        <tr>
          <th className="px-3 py-2.5 text-left text-[11px] font-bold text-text-ghost uppercase tracking-[0.05em] border-b border-border whitespace-nowrap bg-surface-1">
            Time
          </th>
          <th className="px-3 py-2.5 text-left text-[11px] font-bold text-text-ghost uppercase tracking-[0.05em] border-b border-border whitespace-nowrap bg-surface-1">
            Client
          </th>
          <th className="px-3 py-2.5 text-left text-[11px] font-bold text-text-ghost uppercase tracking-[0.05em] border-b border-border whitespace-nowrap bg-surface-1">
            Domain
          </th>
          <th className="px-3 py-2.5 text-center text-[11px] font-bold text-text-ghost uppercase tracking-[0.05em] border-b border-border whitespace-nowrap bg-surface-1">
            Type
          </th>
          <th className="px-3 py-2.5 text-center text-[11px] font-bold text-text-ghost uppercase tracking-[0.05em] border-b border-border whitespace-nowrap bg-surface-1">
            Status
          </th>
          <th className="px-3 py-2.5 text-left text-[11px] font-bold text-text-ghost uppercase tracking-[0.05em] border-b border-border whitespace-nowrap bg-surface-1">
            Reason
          </th>
          {!compact && (
            <th className="px-3 py-2.5 text-center text-[11px] font-bold text-text-ghost uppercase tracking-[0.05em] border-b border-border whitespace-nowrap bg-surface-1 w-16" />
          )}
        </tr>
      </thead>
      <tbody>
        {rows.length === 0 && (
          <tr>
            <td
              colSpan={colSpan}
              className="px-3 py-8 text-[12px] border-b border-border-dim align-middle text-center text-text-dead"
            >
              {loading
                ? "Loading…"
                : totalRows === undefined || totalRows === 0
                  ? "No queries yet"
                  : "No results on this page"}
            </td>
          </tr>
        )}
        {rows.map((r, i) => {
          const conf = r.confidence ?? 0;
          const confPct = Math.round(conf * 100);
          const qtypeLabel = QTYPE_MAP[r.qtype] ?? String(r.qtype);
          const cat = (r.category ?? "").toLowerCase();
          const reasonRaw = (r.reason || r.category || "").toLowerCase();
          const isML =
            reasonRaw.startsWith("ml:") ||
            cat.includes("phishing") ||
            cat.includes("dga") ||
            cat.includes("malware");
          const fbKey = r.domain + "|" + r.timestamp;
          const fbSent = feedbackSent[fbKey];
          const qaState = quickAction[r.domain];
          const label = reasonLabel(r.category, r.reason);
          const reasonColor = label ? reasonBadgeColor(cat, reasonRaw) : null;

          return (
            <tr key={i} style={{ background: rowBg(r.blocked, i) }}>
              {/* time */}
              <td className="px-3 py-2 text-[12px] border-b border-border-dim align-middle whitespace-nowrap">
                <span className="text-text-dim tabular-nums text-[12px]">
                  {formatTime(r.timestamp)}
                </span>
                <br />
                <span className="text-text-dead text-[11px]">
                  {relativeTime(r.timestamp)}
                </span>
              </td>

              {/* client */}
              <td className="px-3 py-2 text-[12px] border-b border-border-dim align-middle whitespace-nowrap">
                {compact ? (
                  <>
                    <span
                      className={`text-[12px] ${r.client_label ? "font-semibold text-[#7a9e88]" : "font-mono text-[#777]"}`}
                    >
                      {r.client_label || r.client_ip}
                    </span>
                    {r.client_label && (
                      <div className="font-mono text-[11px] text-text-dead mt-px">
                        {r.client_ip}
                      </div>
                    )}
                  </>
                ) : (
                  <>
                    <span
                      onClick={() =>
                        navigate(
                          `/clients?edit=${encodeURIComponent(r.client_ip)}`,
                        )
                      }
                      title="Edit client rules"
                      className={`text-[12px] text-[#7a9e88] cursor-pointer underline decoration-[#7a9e8844] underline-offset-[3px] ${r.client_label ? "font-semibold" : "font-mono"}`}
                    >
                      {r.client_label || r.client_ip}
                    </span>
                    {r.client_label && (
                      <div className="font-mono text-[11px] text-text-dead mt-0.5">
                        {r.client_ip}
                      </div>
                    )}
                  </>
                )}
              </td>

              {/* domain */}
              <td
                className="px-3 py-2 text-[13px] border-b border-border-dim align-middle font-mono break-all max-w-65"
                style={{ color: r.blocked ? "#e07070" : "#e0e0e0" }}
              >
                {r.domain}
              </td>

              {/* qtype badge */}
              <td className="px-3 py-2 text-[12px] border-b border-border-dim align-middle text-center">
                <span className="text-[11px] font-bold px-1.75 py-0.5 rounded bg-border text-[#777] border border-[#2e2e2e] font-mono">
                  {qtypeLabel}
                </span>
              </td>

              {/* status */}
              <td className="px-3 py-2 text-[12px] border-b border-border-dim align-middle text-center">
                {r.blocked ? (
                  <div className="inline-flex flex-col items-center gap-1">
                    <span className="text-[11px] font-bold px-2.25 py-0.5 rounded-full bg-[#3a1010] text-[#e07070] border border-[#6a2020] whitespace-nowrap">
                      Blocked
                    </span>
                    {!compact && isML && confPct > 0 && (
                      <div className="flex items-center gap-1.25 w-20">
                        <div className="flex-1 h-0.75 rounded-xs bg-border overflow-hidden">
                          <div
                            className="h-full rounded-xs transition-[width_0.3s]"
                            style={{
                              width: `${confPct}%`,
                              background:
                                confPct > 80
                                  ? "#c0392b"
                                  : confPct > 60
                                    ? "#e67e22"
                                    : "#7080e0",
                            }}
                          />
                        </div>
                        <span className="text-[10px] text-text-faint tabular-nums whitespace-nowrap">
                          {confPct}%
                        </span>
                      </div>
                    )}
                  </div>
                ) : (
                  <span className="text-[11px] font-bold px-2.25 py-0.5 rounded-full bg-[#0f1f0f] text-[#6a9e6a] border border-[#2a4a2a] whitespace-nowrap">
                    Allowed
                  </span>
                )}
              </td>

              {/* reason */}
              <td className="px-3 py-2 text-[12px] border-b border-border-dim align-middle whitespace-nowrap">
                <div className="flex flex-col gap-1 items-start">
                  {label && reasonColor ? (
                    <span
                      className="text-[11px] font-semibold px-2 py-0.5 rounded whitespace-nowrap"
                      style={{
                        background: reasonColor.bg,
                        color: reasonColor.fg,
                        border: `1px solid ${reasonColor.border}`,
                      }}
                    >
                      {label}
                    </span>
                  ) : (
                    <span className="text-[#333] text-[12px]">—</span>
                  )}
                  {!compact && (
                    <div className="flex gap-0.75">
                      {r.blocked && (
                        <button
                          onClick={() => onQuickAllow?.(r.domain)}
                          disabled={qaState === "pending"}
                          title={`Quick-allow ${r.domain} globally`}
                          className="text-[10px] px-1.5 py-px rounded-[3px] border whitespace-nowrap cursor-pointer disabled:cursor-wait"
                          style={{
                            background:
                              qaState === "allowed" ? "#1a2a1a" : "#111",
                            color:
                              qaState === "allowed" ? "#80c080" : "#798777",
                            borderColor: "#2a3a2a",
                            opacity: qaState === "pending" ? 0.6 : 1,
                          }}
                        >
                          {qaState === "allowed"
                            ? "✓ Allowed"
                            : qaState === "pending"
                              ? "…"
                              : "✓ Allow"}
                        </button>
                      )}
                      {!r.blocked && (
                        <button
                          onClick={() => onQuickBlock?.(r.domain)}
                          disabled={qaState === "pending"}
                          title={`Quick-block ${r.domain} globally`}
                          className="text-[10px] px-1.5 py-px rounded-[3px] border whitespace-nowrap cursor-pointer disabled:cursor-wait"
                          style={{
                            background:
                              qaState === "blocked" ? "#2a1010" : "#111",
                            color:
                              qaState === "blocked" ? "#e07070" : "#a06060",
                            borderColor: "#3a2020",
                            opacity: qaState === "pending" ? 0.6 : 1,
                          }}
                        >
                          {qaState === "blocked"
                            ? "✓ Blocked"
                            : qaState === "pending"
                              ? "…"
                              : "✗ Block"}
                        </button>
                      )}
                    </div>
                  )}
                </div>
              </td>

              {/* feedback — full mode only */}
              {!compact && (
                <td className="px-3 py-2 text-[12px] border-b border-border-dim align-middle text-center whitespace-nowrap">
                  {isML ? (
                    fbSent ? (
                      <span
                        className="text-[11px]"
                        style={{
                          color: fbSent === "safe" ? ACCENT : "#c0392b",
                        }}
                      >
                        {fbSent === "safe" ? "✓ Safe" : "✓ Malicious"}
                      </span>
                    ) : (
                      <div className="flex gap-1 justify-center">
                        <button
                          onClick={() => onFeedback?.(r, "safe")}
                          title="Mark as safe (false positive)"
                          className="flex items-center justify-center w-6 h-6 p-0 bg-transparent border border-[#2a4a2a] rounded text-[#4a8a4a] cursor-pointer text-[11px]"
                        >
                          <FaThumbsUp />
                        </button>
                        <button
                          onClick={() => onFeedback?.(r, "malicious")}
                          title="Confirm as malicious"
                          className="flex items-center justify-center w-6 h-6 p-0 bg-transparent border border-[#4a2a2a] rounded text-[#8a4a4a] cursor-pointer text-[11px]"
                        >
                          <FaThumbsDown />
                        </button>
                      </div>
                    )
                  ) : (
                    <span className="text-border-mid text-[12px]">—</span>
                  )}
                </td>
              )}
            </tr>
          );
        })}
      </tbody>
    </table>
  );
};

export default QueryTable;
