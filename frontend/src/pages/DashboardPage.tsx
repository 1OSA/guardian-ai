import React, { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import axios from "axios";
import { FaServer } from "react-icons/fa";
import { ResponsiveContainer, PieChart, Pie, Cell, Tooltip } from "recharts";
import { useAuth } from "../lib/AuthContext";
import { useMediaQuery } from "../lib/useMediaQuery";
import {
  ACCENT,
  POLL_DASHBOARD_MS,
  POLL_QUERIES_LIVE_MS,
  POLL_RELATIVE_TIME_MS,
} from "../lib/constants";
import {
  normalizeBlockReason,
  reasonBadgeColor,
  reasonLabel,
} from "../lib/utils";
import QueryTable from "../components/QueryTable";
import type { QueryRow } from "../lib/types";

/* ── Shared sub-components ─────────────────────────────────────────────── */

const StatCard: React.FC<{
  label: string;
  value: string;
  valueColor?: string;
  sub: string;
}> = ({ label, value, valueColor = "var(--color-text)", sub }) => (
  <div className="bg-surface-1 text-text rounded-[10px] border border-border shadow-card px-5 py-4">
    <div className="text-[11px] font-semibold text-text-ghost uppercase tracking-[0.05em] mb-1.5">
      {label}
    </div>
    <div
      className="text-[30px] font-bold leading-none"
      style={{ color: valueColor }}
    >
      {value}
    </div>
    <div className="text-[11px] text-text-dead mt-1.5">{sub}</div>
  </div>
);

const ListHeader: React.FC<{ title: string }> = ({ title }) => (
  <div className="px-4 pt-3.5 pb-2.5 text-[12px] font-bold text-text-faint uppercase tracking-[0.05em] border-b border-border">
    {title}
  </div>
);

type StatsData = {
  total: number;
  blocked: number;
  ml_blocked: number;
  total_24h: number;
  blocked_24h: number;
  top_domains: { domain: string; count: number }[];
  top_blocked: { domain: string; count: number }[];
  qtype_breakdown: { qtype: number; label: string; count: number }[];
  cat_breakdown: { category: string; count: number }[];
  block_reasons: { reason: string; count: number }[];
  ml_enabled: boolean;
  ml_connected: boolean;
};

const DashboardPage: React.FC = () => {
  useAuth();
  const navigate = useNavigate();
  const isMobile = useMediaQuery("(max-width: 768px)");

  const [stats, setStats] = useState<StatsData | null>(null);
  const [recent, setRecent] = useState<QueryRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [, setTick] = useState(0);

  const fetchStats = React.useCallback(async () => {
    try {
      const st = await axios.get("/api/stats");
      setStats(st.data as StatsData);
    } catch {
      /* ignore */
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchStats();
    const iv = setInterval(() => {
      if (!document.hidden) fetchStats();
    }, POLL_DASHBOARD_MS);
    return () => clearInterval(iv);
  }, [fetchStats]);

  const fetchRecent = React.useCallback(async () => {
    try {
      const res = await axios.get("/api/queries?limit=20");
      console.log("[fetchRecent] API response:", res.data);
      if (res.data && res.data.length > 0) {
        console.log(
          "[fetchRecent] First item timestamp:",
          res.data[0].timestamp,
        );
        console.log("[fetchRecent] All fields:", Object.keys(res.data[0]));
      }
      setRecent(res.data as QueryRow[]);
    } catch (err) {
      console.error("[fetchRecent] Error:", err);
    }
  }, []);

  useEffect(() => {
    fetchRecent();
    const iv = setInterval(() => {
      if (!document.hidden) fetchRecent();
    }, POLL_QUERIES_LIVE_MS);
    return () => clearInterval(iv);
  }, [fetchRecent]);

  useEffect(() => {
    const t = setInterval(() => {
      if (!document.hidden) setTick((n) => n + 1);
    }, POLL_RELATIVE_TIME_MS);
    return () => clearInterval(t);
  }, []);

  const total = stats?.total ?? 0;
  const blocked = stats?.blocked ?? 0;
  const allowed = total - blocked;
  const blockRate = total > 0 ? (blocked / total) * 100 : 0;
  const mlBlocked = stats?.ml_blocked ?? 0;
  const total24h = stats?.total_24h ?? 0;
  const blocked24h = stats?.blocked_24h ?? 0;

  const groupedBlockReasons = React.useMemo(() => {
    const counts = new Map<string, number>();
    for (const row of stats?.block_reasons ?? []) {
      const key = normalizeBlockReason(row.reason) || "unknown";
      counts.set(key, (counts.get(key) ?? 0) + row.count);
    }
    return Array.from(counts.entries())
      .map(([reason, count]) => ({ reason, count }))
      .sort((a, b) => b.count - a.count);
  }, [stats?.block_reasons]);

  const pieData = [
    { name: "Allowed", value: allowed, color: "#4a7a4a" },
    { name: "Blocked", value: blocked, color: "#8b3030" },
  ];

  if (loading) {
    return (
      <div className="flex items-center justify-center h-50 text-text-dead text-[14px]">
        Loading…
      </div>
    );
  }

  return (
    <div className={isMobile ? "p-3" : "p-6"}>
      {/* ── page title ── */}
      <div className="flex items-center justify-between mb-5">
        <div className="flex items-center gap-2">
          <FaServer style={{ color: ACCENT, fontSize: 18 }} />
          <span className="text-xl font-bold text-text">Dashboard</span>
        </div>
        <div className="flex items-center gap-2">
          {/* ML status pill */}
          <span
            className="flex items-center gap-1.25 text-[11px] font-semibold px-2.5 py-0.75 rounded-full border"
            style={{
              background: stats?.ml_connected
                ? "var(--color-success-dim)"
                : "var(--color-danger-dim)",
              color: stats?.ml_connected
                ? "var(--color-success)"
                : "var(--color-danger)",
              borderColor: stats?.ml_connected
                ? "var(--color-success-border)"
                : "var(--color-danger-border)",
            }}
          >
            <span
              className="w-1.5 h-1.5 rounded-full shrink-0"
              style={{
                background: stats?.ml_connected
                  ? "var(--color-success)"
                  : "var(--color-danger)",
                boxShadow: stats?.ml_connected
                  ? "0 0 5px var(--color-success)"
                  : "none",
              }}
            />
            ML {stats?.ml_connected ? "Connected" : "Offline"}
          </span>
          <button
            onClick={() => {
              fetchStats();
              fetchRecent();
            }}
            title="Refresh stats and recent queries"
            className="px-2.75 py-1.25 bg-surface-1 text-text-faint border border-border-mid rounded-md cursor-pointer text-[13px]"
          >
            ↻
          </button>
        </div>
      </div>

      {/* ── stat cards ── */}
      <div
        className={`grid gap-3 mb-4 ${isMobile ? "grid-cols-2" : "grid-cols-4"}`}
      >
        <StatCard
          label="Total Queries"
          value={total.toLocaleString()}
          sub={`${total24h.toLocaleString()} in last 24h`}
        />
        <StatCard
          label="Allowed"
          value={allowed.toLocaleString()}
          valueColor="var(--color-success)"
          sub={`${total24h > 0 ? (total24h - blocked24h).toLocaleString() : "0"} in last 24h`}
        />
        <StatCard
          label="Blocked"
          value={blocked.toLocaleString()}
          valueColor="var(--color-danger)"
          sub={`${blocked24h.toLocaleString()} in last 24h`}
        />
        <StatCard
          label="Block Rate"
          value={`${blockRate.toFixed(1)}%`}
          valueColor={
            blockRate > 20 ? "var(--color-warn)" : "var(--color-text)"
          }
          sub={`${mlBlocked.toLocaleString()} blocked by ML`}
        />
      </div>

      {/* ── middle row: pie + top domains + top blocked ── */}
      <div
        className={`grid gap-3 mb-4 items-start ${
          isMobile ? "grid-cols-1" : "grid-cols-[220px_1fr_1fr]"
        }`}
      >
        {/* donut chart */}
        <div className="bg-surface-1 text-text rounded-[10px] border border-border shadow-card px-5 py-4">
          <div className="text-[12px] font-bold text-text-faint uppercase tracking-[0.05em] mb-3">
            Traffic Split
          </div>
          <ResponsiveContainer width="100%" height={160}>
            <PieChart>
              <Pie
                data={pieData}
                cx="50%"
                cy="50%"
                innerRadius={42}
                outerRadius={62}
                dataKey="value"
                animationBegin={0}
                animationDuration={600}
                stroke="none"
              >
                {pieData.map((entry, i) => (
                  <Cell key={i} fill={entry.color} />
                ))}
              </Pie>
              <Tooltip
                contentStyle={{
                  background: "var(--color-surface-1)",
                  border: "1px solid var(--color-border-mid)",
                  borderRadius: 6,
                  fontSize: 12,
                }}
                itemStyle={{ color: "var(--color-text-dim)" }}
                formatter={(v) => [Number(v as number).toLocaleString(), ""]}
              />
            </PieChart>
          </ResponsiveContainer>
          <div className="flex justify-center gap-3.5 mt-1">
            {pieData.map((e) => (
              <div
                key={e.name}
                className="flex items-center gap-1.25 text-[11px] text-text-muted"
              >
                <span
                  className="w-2 h-2 rounded-xs inline-block"
                  style={{ background: e.color }}
                />
                {e.name}
              </div>
            ))}
          </div>
        </div>

        {/* top queried domains */}
        <div className="bg-surface-1 text-text rounded-[10px] border border-border shadow-card overflow-hidden">
          <ListHeader title="Top Queried Domains" />
          {(stats?.top_domains ?? []).slice(0, 8).map((d, i) => {
            const pct = stats?.top_domains?.[0]?.count
              ? (d.count / stats.top_domains[0].count) * 100
              : 0;
            return (
              <div
                key={d.domain}
                onClick={() =>
                  navigate(`/queries?q=${encodeURIComponent(d.domain)}`)
                }
                className="flex items-center gap-2.5 px-4 py-1.75 border-b border-surface-1 cursor-pointer"
              >
                <span className="text-[11px] text-text-dead w-4 text-right shrink-0">
                  {i + 1}
                </span>
                <div className="flex-1 min-w-0">
                  <div className="text-[12px] font-mono text-accent overflow-hidden text-ellipsis whitespace-nowrap">
                    {d.domain}
                  </div>
                  <div className="h-0.75 rounded-xs bg-border mt-1 overflow-hidden">
                    <div
                      className="h-full rounded-xs"
                      style={{ width: `${pct}%`, background: ACCENT }}
                    />
                  </div>
                </div>
                <span className="text-[12px] text-text-faint shrink-0 tabular-nums">
                  {d.count.toLocaleString()}
                </span>
              </div>
            );
          })}
          {(stats?.top_domains ?? []).length === 0 && (
            <div className="p-6 text-center text-text-dead text-[13px]">
              No data
            </div>
          )}
        </div>

        {/* top blocked + category breakdown */}
        <div className="flex flex-col gap-3">
          {/* top blocked */}
          <div className="bg-surface-1 text-text rounded-[10px] border border-border shadow-card overflow-hidden">
            <ListHeader title="Top Blocked" />
            {(stats?.top_blocked ?? []).slice(0, 5).map((d, i) => (
              <div
                key={d.domain}
                onClick={() =>
                  navigate(
                    `/queries?q=${encodeURIComponent(d.domain)}&blocked=1`,
                  )
                }
                className="flex items-center gap-2.5 px-4 py-1.75 border-b border-surface-1 cursor-pointer"
              >
                <span className="text-[11px] text-text-dead w-4 text-right shrink-0">
                  {i + 1}
                </span>
                <span className="flex-1 text-[12px] font-mono text-danger overflow-hidden text-ellipsis whitespace-nowrap min-w-0 underline decoration-danger-border underline-offset-2">
                  {d.domain}
                </span>
                <span className="text-[12px] text-text-faint shrink-0 tabular-nums">
                  {d.count.toLocaleString()}
                </span>
              </div>
            ))}
            {(stats?.top_blocked ?? []).length === 0 && (
              <div className="p-4 text-center text-text-dead text-[13px]">
                No blocks yet
              </div>
            )}
          </div>

          {/* block reasons */}
          <div className="bg-surface-1 text-text rounded-[10px] border border-border shadow-card overflow-hidden">
            <ListHeader title="Block Reasons" />
            {groupedBlockReasons.map((b) => {
              const colors = reasonBadgeColor(b.reason, b.reason) ?? {
                bg: "var(--color-success-dim)",
                fg: "var(--color-success-text)",
                border: "var(--color-success-border)",
              };
              const { fg: color, bg } = colors;
              return (
                <div
                  key={b.reason}
                  className="flex items-center justify-between px-4 py-1.75 border-b border-surface-1"
                >
                  <span
                    className="text-[11px] font-semibold px-2 py-0.5 rounded"
                    style={{
                      background: bg,
                      color,
                      border: `1px solid ${colors.border}`,
                    }}
                  >
                    {reasonLabel(b.reason, b.reason) ?? b.reason}
                  </span>
                  <span className="text-[12px] text-text-faint tabular-nums">
                    {b.count.toLocaleString()}
                  </span>
                </div>
              );
            })}
            {groupedBlockReasons.length === 0 && (
              <div className="p-4 text-center text-text-dead text-[13px]">
                No blocks yet
              </div>
            )}
          </div>
        </div>
      </div>

      {/* ── recent queries ── */}
      <div className="bg-surface-1 text-text rounded-[10px] border border-border shadow-card overflow-hidden">
        <div className="px-5 pt-3.5 pb-3 border-b border-border flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-[12px] font-bold text-text-faint uppercase tracking-[0.05em]">
              Recent Queries
            </span>
            <span
              title="Auto-refreshes every 3 seconds"
              className="inline-flex items-center gap-1 text-[11px] text-success"
            >
              <span
                className="w-1.5 h-1.5 rounded-full inline-block shrink-0"
                style={{ background: ACCENT, boxShadow: `0 0 5px ${ACCENT}` }}
              />
              Live
            </span>
          </div>
          <a
            href="/queries"
            className="text-[12px] no-underline"
            style={{ color: ACCENT }}
          >
            View all →
          </a>
        </div>
        <div className="overflow-x-auto">
          <QueryTable compact rows={recent} />
        </div>
      </div>
    </div>
  );
};

export default DashboardPage;
