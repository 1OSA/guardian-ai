import React, { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import axios from "axios";
import { FaFilter } from "react-icons/fa";
import { useMediaQuery } from "../lib/useMediaQuery";
import {
  ACCENT,
  PAGE_SIZE,
  POLL_QUERIES_LIVE_MS,
  POLL_RELATIVE_TIME_MS,
} from "../lib/constants";
import QueryTable from "../components/QueryTable";
import type { QueryRow } from "../lib/types";

const QueriesPage: React.FC = () => {
  const [rows, setRows] = useState<QueryRow[]>([]);
  const [total, setTotal] = useState(0);
  const location = useLocation();
  const initParams = new URLSearchParams(location.search);
  const [q, setQ] = useState(initParams.get("q") ?? "");
  const [blockedOnly, setBlockedOnly] = useState(
    initParams.get("blocked") === "1",
  );
  const [typeFilter, setTypeFilter] = useState("ALL");
  const [loading, setLoading] = useState(false);
  const [live, setLive] = useState(false);
  const [, setTick] = useState(0);
  const [page, setPage] = useState(0);
  const liveRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const isMobile = useMediaQuery("(max-width: 768px)");
  const [feedbackSent, setFeedbackSent] = useState<Record<string, string>>({});
  const [quickAction, setQuickAction] = useState<Record<string, string>>({});

  const sendFeedback = async (r: QueryRow, verdict: "safe" | "malicious") => {
    const key = r.domain + "|" + r.timestamp;
    if (feedbackSent[key]) return;
    try {
      await axios.post("/api/ml/feedback", {
        domain: r.domain,
        verdict,
        category: r.category ?? "",
        confidence: r.confidence ?? 0,
        client_ip: r.client_ip,
      });
      setFeedbackSent((prev) => ({ ...prev, [key]: verdict }));
    } catch {
      /* ignore */
    }
  };

  const quickAllow = async (domain: string) => {
    if (quickAction[domain] === "pending") return;
    setQuickAction((prev) => ({ ...prev, [domain]: "pending" }));
    try {
      await axios.post("/api/queries/allow", { domain });
      setQuickAction((prev) => ({ ...prev, [domain]: "allowed" }));
      setRows((prev) =>
        prev.map((r) =>
          r.domain === domain ? { ...r, blocked: false, reason: "allowed" } : r,
        ),
      );
    } catch {
      setQuickAction((prev) => ({ ...prev, [domain]: "" }));
    }
  };

  const quickBlock = async (domain: string) => {
    if (quickAction[domain] === "pending") return;
    setQuickAction((prev) => ({ ...prev, [domain]: "pending" }));
    try {
      await axios.post("/api/queries/block", { domain });
      setQuickAction((prev) => ({ ...prev, [domain]: "blocked" }));
      setRows((prev) =>
        prev.map((r) =>
          r.domain === domain
            ? {
                ...r,
                blocked: true,
                reason: "blocklist",
                category: "blocklist",
              }
            : r,
        ),
      );
    } catch {
      setQuickAction((prev) => ({ ...prev, [domain]: "" }));
    }
  };

  const loadRows = React.useCallback(
    async (
      opts: {
        filter?: string;
        pg?: number;
        blocked?: boolean;
        type?: string;
      } = {},
    ) => {
      const filter = opts.filter ?? q;
      const pg = opts.pg ?? page;
      const blk = opts.blocked ?? blockedOnly;
      const typ = opts.type ?? typeFilter;
      setLoading(true);
      try {
        const params = new URLSearchParams();
        if (filter) params.set("q", filter);
        params.set("limit", String(PAGE_SIZE));
        params.set("offset", String(pg * PAGE_SIZE));
        if (blk) params.set("blocked", "1");
        if (typ !== "ALL") params.set("type", typ);
        const res = await axios.get("/api/queries?" + params.toString());
        setRows(res.data as QueryRow[]);
        const hdr = res.headers["x-total-count"];
        setTotal(
          hdr !== undefined
            ? parseInt(hdr, 10)
            : (res.data as QueryRow[]).length,
        );
      } catch {
        /* ignore */
      } finally {
        setLoading(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  useEffect(() => {
    const t = setInterval(() => {
      if (!document.hidden) setTick((n) => n + 1);
    }, POLL_RELATIVE_TIME_MS);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    loadRows({ filter: q, pg: page, blocked: blockedOnly, type: typeFilter });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, q, blockedOnly, typeFilter]);

  useEffect(() => {
    if (live) {
      liveRef.current = setInterval(() => {
        if (!document.hidden)
          loadRows({
            filter: q,
            pg: 0,
            blocked: blockedOnly,
            type: typeFilter,
          });
      }, POLL_QUERIES_LIVE_MS);
    } else {
      if (liveRef.current) clearInterval(liveRef.current);
    }
    return () => {
      if (liveRef.current) clearInterval(liveRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [live, q, blockedOnly, typeFilter]);

  const prevFilters = useRef({ q, blockedOnly, typeFilter });
  useEffect(() => {
    const prev = prevFilters.current;
    if (
      prev.q !== q ||
      prev.blockedOnly !== blockedOnly ||
      prev.typeFilter !== typeFilter
    ) {
      prevFilters.current = { q, blockedOnly, typeFilter };
      setPage(0);
    }
  }, [q, blockedOnly, typeFilter]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const availableTypes = [
    "ALL",
    "A",
    "AAAA",
    "CNAME",
    "MX",
    "TXT",
    "PTR",
    "NS",
    "SRV",
  ];

  const pageBtnCls = (disabled: boolean) =>
    `px-2.5 py-1 bg-transparent border border-border-mid rounded-md text-[12px] cursor-pointer ${
      disabled ? "text-[#333] cursor-default" : "text-text-muted"
    }`;

  return (
    <div className={isMobile ? "p-3" : "p-6"}>
      {/* ── page header ── */}
      <div className="flex items-center justify-between mb-4 flex-wrap gap-2.5">
        <div className="flex items-center gap-2">
          <FaFilter style={{ color: ACCENT, fontSize: 16 }} />
          <span className="text-xl font-bold text-text">Query Log</span>
          <span className="text-[12px] text-text-ghost ml-1">
            {rows.length} / {total}
          </span>
        </div>
        <div className="flex items-center gap-2.5">
          {/* live toggle */}
          <button
            onClick={() => setLive((v) => !v)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-[12px] font-semibold cursor-pointer"
            style={{
              background: live ? "#1e2a1e" : "#1a1a1a",
              color: live ? ACCENT : "#666",
              border: `1px solid ${live ? ACCENT : "#333"}`,
            }}
          >
            <span
              className="w-1.75 h-1.75 rounded-full inline-block"
              style={{
                background: live ? ACCENT : "#444",
                boxShadow: live ? `0 0 6px ${ACCENT}` : "none",
              }}
            />
            Live
          </button>
          {/* export */}
          <a
            href="/queries/export?limit=10000"
            className="px-3 py-1.5 bg-surface-1 text-[#aaa] border border-[#333] rounded-md no-underline text-[12px] font-semibold"
          >
            ↓ Export CSV
          </a>
        </div>
      </div>

      {/* ── filters bar ── */}
      <div className="flex flex-wrap gap-2 mb-3.5 items-center">
        {/* search */}
        <div className="relative flex-[1_1_200px] min-w-40">
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                setPage(0);
                loadRows({
                  filter: e.currentTarget.value,
                  pg: 0,
                  blocked: blockedOnly,
                  type: typeFilter,
                });
              }
            }}
            placeholder="Search domain or client IP…"
            className="w-full box-border py-1.75 pl-8 pr-2.5 bg-surface-2 border border-border-mid rounded-md text-white text-[13px] outline-none"
          />
          <span className="absolute left-2.5 top-1/2 -translate-y-1/2 text-text-ghost text-[13px] pointer-events-none">
            ⌕
          </span>
        </div>

        {/* search btn */}
        <button
          onClick={() =>
            loadRows({
              filter: q,
              pg: page,
              blocked: blockedOnly,
              type: typeFilter,
            })
          }
          className="px-3.5 py-1.75 text-white border-none rounded-md cursor-pointer text-[13px] font-semibold"
          style={{ background: ACCENT }}
        >
          Search
        </button>

        {/* blocked only */}
        <label
          className="flex items-center gap-1.5 cursor-pointer text-[13px] select-none px-3 py-1.5 rounded-md"
          style={{
            color: blockedOnly ? "#e07070" : "#888",
            background: blockedOnly ? "#2a1a1a" : "#1a1a1a",
            border: `1px solid ${blockedOnly ? "#7a3333" : "#2a2a2a"}`,
          }}
        >
          <input
            type="checkbox"
            checked={blockedOnly}
            onChange={(e) => setBlockedOnly(e.target.checked)}
            style={{ accentColor: "#c0392b" }}
          />
          Blocked only
        </label>

        {/* type filter */}
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="py-1.75 px-2.5 bg-surface-2 border border-border-mid rounded-md text-[#aaa] text-[13px] cursor-pointer outline-none"
        >
          {availableTypes.map((t) => (
            <option key={t} value={t}>
              {t === "ALL" ? "All types" : t}
            </option>
          ))}
        </select>

        {/* refresh */}
        <button
          onClick={() => {
            setPage(0);
            loadRows({
              filter: q,
              pg: 0,
              blocked: blockedOnly,
              type: typeFilter,
            });
          }}
          disabled={loading}
          className="px-3 py-1.75 bg-surface-1 border border-border-mid rounded-md text-[13px] cursor-pointer disabled:cursor-default"
          style={{ color: loading ? "#444" : "#888" }}
        >
          {loading ? "…" : "↻"}
        </button>
      </div>

      {/* ── table ── */}
      <div className="bg-surface-1 border border-border-mid rounded-xl overflow-hidden shadow-[0_2px_12px_rgba(0,0,0,0.35)]">
        <div className="overflow-x-auto">
          <QueryTable
            rows={rows}
            loading={loading}
            totalRows={total}
            quickAction={quickAction}
            feedbackSent={feedbackSent}
            onQuickAllow={quickAllow}
            onQuickBlock={quickBlock}
            onFeedback={sendFeedback}
          />
        </div>
      </div>

      {/* ── pagination ── */}
      {total > 0 && (
        <div className="flex items-center justify-between mt-3 flex-wrap gap-2">
          <span className="text-[12px] text-text-ghost">
            {total.toLocaleString()} results &nbsp;·&nbsp; page{" "}
            <strong className="text-text-muted">{page + 1}</strong> of{" "}
            {totalPages}
          </span>
          <div className="flex gap-1.5">
            <button
              onClick={() => setPage(0)}
              disabled={page === 0}
              className={pageBtnCls(page === 0)}
            >
              «
            </button>
            <button
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              disabled={page === 0}
              className={pageBtnCls(page === 0)}
            >
              ‹ Prev
            </button>
            <button
              onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
              disabled={page >= totalPages - 1}
              className={pageBtnCls(page >= totalPages - 1)}
            >
              Next ›
            </button>
            <button
              onClick={() => setPage(totalPages - 1)}
              disabled={page >= totalPages - 1}
              className={pageBtnCls(page >= totalPages - 1)}
            >
              »
            </button>
          </div>
        </div>
      )}
    </div>
  );
};

export default QueriesPage;
