import React, { useEffect, useRef, useState } from "react";
import axios from "axios";
import { FaCog, FaServer, FaUser, FaFilter, FaSyncAlt } from "react-icons/fa";
import { useAuth } from "../lib/AuthContext";
import { useMediaQuery } from "../lib/useMediaQuery";
import { ACCENT } from "../lib/constants";
import RulesErrors from "../components/RulesErrors";
import ToggleSwitch from "../components/ToggleSwitch";
import DNSModal from "../components/DNSModal";
import DomainTestPanel from "../components/DomainTestPanel";
import type { DomainTestResult } from "../components/DomainTestPanel";

const SettingsPage: React.FC = () => {
  const { user } = useAuth();
  const isMobile = useMediaQuery("(max-width: 768px)");

  const [blocklist, setBlocklist] = useState<string[]>([]);
  const [predefined, setPredefined] = useState<{ name: string; url: string }[]>(
    [],
  );
  const [activeSources, setActiveSources] = useState<Set<string>>(new Set());
  const [sourceLoading, setSourceLoading] = useState<string | null>(null);
  const [customURLs, setCustomURLs] = useState<string>("");
  const [urlsSaveStatus, setUrlsSaveStatus] = useState<
    "idle" | "saving" | "saved" | "error"
  >("idle");
  const urlsDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const urlsInitialized = useRef(false);

  const [customRules, setCustomRules] = useState<string>("");
  const [rulesSaveStatus, setRulesSaveStatus] = useState<
    "idle" | "saving" | "saved" | "error"
  >("idle");
  const rulesDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const rulesInitialized = useRef(false);

  const [loading, setLoading] = useState(false);
  const [reloadLoading, setReloadLoading] = useState(false);
  const [upstream, setUpstream] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [pwChanged, setPwChanged] = useState(false);
  const [blocklistEnabled, setBlocklistEnabled] = useState(true);

  const [upstreamSaveStatus, setUpstreamSaveStatus] = useState<
    "idle" | "saving" | "saved" | "error"
  >("idle");
  const upstreamDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const upstreamInitialized = useRef(false);

  const [retentionInput, setRetentionInput] = useState<string>("90");
  const [retentionStatus, setRetentionStatus] = useState<
    "" | "saving" | "saved" | "error"
  >("");
  const retentionDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const retentionInitialized = useRef(false);

  const [testDomain, setTestDomain] = useState("");
  const [testClient, setTestClient] = useState("");
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<DomainTestResult | null>(null);
  const [testError, setTestError] = useState<string | null>(null);

  const [showDNSModal, setShowDNSModal] = useState(false);
  const [modalIP, setModalIP] = useState("");
  const [modalInitialOS, setModalInitialOS] = useState("windows");

  const upstreamPresets = [
    "8.8.8.8",
    "8.8.4.4",
    "1.1.1.1",
    "1.0.0.1",
    "9.9.9.9",
    "208.67.222.222",
    "208.67.220.220",
  ];

  // ── data fetchers ────────────────────────────────────────────────────────

  const fetchUpstream = async () => {
    try {
      const res = await axios.get("/api/upstream");
      setUpstream((res.data as { servers: string[] }).servers.join("\n"));
      upstreamInitialized.current = true;
    } catch {
      /* ignore */
    }
  };

  const saveUpstream = async (value: string) => {
    setUpstreamSaveStatus("saving");
    try {
      const servers = value
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean);
      await axios.post("/api/upstream", { servers });
      setUpstreamSaveStatus("saved");
      setTimeout(() => setUpstreamSaveStatus("idle"), 2000);
    } catch {
      setUpstreamSaveStatus("error");
      setTimeout(() => setUpstreamSaveStatus("idle"), 3000);
    }
  };

  const handleUpstreamChange = (value: string) => {
    setUpstream(value);
    if (!upstreamInitialized.current) return;
    if (upstreamDebounceRef.current) clearTimeout(upstreamDebounceRef.current);
    upstreamDebounceRef.current = setTimeout(() => saveUpstream(value), 800);
  };

  const fetchBlocklist = async () => {
    try {
      const res = await axios.get("/api/blocklist/stats");
      setBlocklist(new Array(res.data.total_entries));
    } catch {
      /* ignore */
    }
  };

  const saveCustomURLs = async (value: string) => {
    setUrlsSaveStatus("saving");
    try {
      const urls = value
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean);
      const sourcesRes = await axios.get("/api/blocklist/sources");
      const predefinedURLs = predefined.map((p) => p.url);
      const existing = (
        sourcesRes.data as { name: string; url: string }[]
      ).filter((s) => predefinedURLs.includes(s.url));
      const data = [...existing, ...urls.map((url) => ({ url, name: url }))];
      await axios.post("/api/blocklist/sources", data);
      setUrlsSaveStatus("saved");
      setTimeout(() => setUrlsSaveStatus("idle"), 2000);
    } catch {
      setUrlsSaveStatus("error");
      setTimeout(() => setUrlsSaveStatus("idle"), 3000);
    }
  };

  const handleURLsChange = (value: string) => {
    setCustomURLs(value);
    if (!urlsInitialized.current) return;
    if (urlsDebounceRef.current) clearTimeout(urlsDebounceRef.current);
    urlsDebounceRef.current = setTimeout(() => saveCustomURLs(value), 900);
  };

  const fetchCustomRules = async () => {
    try {
      const res = await axios.get("/api/rules");
      setCustomRules((res.data as { rules: string }).rules ?? "");
      rulesInitialized.current = true;
    } catch {
      /* ignore */
    }
  };

  const saveCustomRules = async (value: string) => {
    setRulesSaveStatus("saving");
    try {
      const rules = value;
      await axios.post("/api/rules", { rules });
      setRulesSaveStatus("saved");
      setTimeout(() => setRulesSaveStatus("idle"), 2000);
    } catch {
      setRulesSaveStatus("error");
      setTimeout(() => setRulesSaveStatus("idle"), 3000);
    }
  };

  const handleRulesChange = (value: string) => {
    setCustomRules(value);
    if (!rulesInitialized.current) return;
    if (rulesDebounceRef.current) clearTimeout(rulesDebounceRef.current);
    rulesDebounceRef.current = setTimeout(() => saveCustomRules(value), 800);
  };

  const fetchActiveSources = async () => {
    try {
      const res = await axios.get("/api/blocklist/sources");
      const active = new Set((res.data as { url: string }[]).map((s) => s.url));
      setActiveSources(active);
    } catch {
      /* ignore */
    }
  };

  const fetchToggles = async () => {
    try {
      const blRes = await axios.get("/api/blocklist/enabled");
      setBlocklistEnabled((blRes.data as { enabled: boolean }).enabled ?? true);
    } catch {
      /* ignore */
    }
  };

  const fetchRetention = async () => {
    try {
      const res = await axios.get("/api/retention");
      setRetentionInput(String((res.data as { days: number }).days ?? 90));
      retentionInitialized.current = true;
    } catch {
      /* ignore */
    }
  };

  const saveRetention = async (value: string) => {
    const days = parseInt(value, 10);
    if (isNaN(days) || days < 1) return;
    setRetentionStatus("saving");
    try {
      await axios.post("/api/retention", { days });
      setRetentionStatus("saved");
      setTimeout(() => setRetentionStatus(""), 2000);
    } catch {
      setRetentionStatus("error");
      setTimeout(() => setRetentionStatus(""), 3000);
    }
  };

  const handleRetentionChange = (value: string) => {
    setRetentionInput(value);
    if (!retentionInitialized.current) return;
    if (retentionDebounceRef.current)
      clearTimeout(retentionDebounceRef.current);
    retentionDebounceRef.current = setTimeout(() => saveRetention(value), 800);
  };

  const init = async () => {
    const loadedPredefined = await axios
      .get("/api/blocklist/predefined")
      .then((r) => r.data as { name: string; url: string }[])
      .catch(() => [] as { name: string; url: string }[]);
    setPredefined(loadedPredefined);
    await Promise.all([
      fetchUpstream(),
      fetchBlocklist(),
      fetchActiveSources(),
      fetchCustomRules(),
      fetchToggles(),
      fetchRetention(),
    ]);
    // fetchCustomURLs needs predefined loaded first
    try {
      const res = await axios.get("/api/blocklist/sources");
      const sources = res.data as { name: string; url: string }[];
      const predefinedURLs = loadedPredefined.map((p) => p.url);
      const customLines = sources
        .filter((s) => !predefinedURLs.includes(s.url))
        .map((s) => s.url);
      setCustomURLs(customLines.join("\n"));
      urlsInitialized.current = true;
    } catch {
      /* ignore */
    }
  };

  useEffect(() => {
    init();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const clearBlocklist = async () => {
    if (!confirm("Clear the entire blocklist? This cannot be undone.")) return;
    setLoading(true);
    try {
      await axios.delete("/api/blocklist");
      await fetchBlocklist();
    } catch {
      /* ignore */
    } finally {
      setLoading(false);
    }
  };

  const togglePredefined = async (item: { name: string; url: string }) => {
    const isActive = activeSources.has(item.url);
    setSourceLoading(item.url);
    try {
      const sourcesRes = await axios.get("/api/blocklist/sources");
      const current = sourcesRes.data as { name: string; url: string }[];
      const data = isActive
        ? current.filter((s) => s.url !== item.url)
        : [...current, { name: item.name, url: item.url }];
      await axios.post("/api/blocklist/sources", data);
      const next = new Set(activeSources);
      if (isActive) {
        next.delete(item.url);
      } else {
        next.add(item.url);
      }
      setActiveSources(next);
      const sources: { name: string; url: string }[] = data;
      await axios.post("/api/blocklist/sources/reload", { sources });
      await fetchBlocklist();
    } catch {
      /* ignore */
    } finally {
      setSourceLoading(null);
    }
  };

  const toggleBlocklist = async (val: boolean) => {
    setBlocklistEnabled(val);
    try {
      await axios.post("/api/blocklist/enabled", { enabled: val });
    } catch {
      /* ignore */
    }
  };

  const reloadSources = async () => {
    setReloadLoading(true);
    try {
      const sourcesRes = await axios.get("/api/blocklist/sources");
      const sources = sourcesRes.data as { name: string; url: string }[];
      await axios.post("/api/blocklist/sources/reload", { sources });
      await fetchBlocklist();
    } catch {
      /* ignore */
    } finally {
      setReloadLoading(false);
    }
  };

  const changePassword = async () => {
    if (!newPassword) return;
    try {
      const password = newPassword;
      await axios.post("/api/change-password", { password });
      setNewPassword("");
      setPwChanged(true);
      setTimeout(() => setPwChanged(false), 3000);
    } catch {
      /* ignore */
    }
  };

  const runDomainTest = async () => {
    const d = testDomain.trim();
    if (!d) return;
    setTestLoading(true);
    setTestResult(null);
    setTestError(null);
    try {
      const params: Record<string, string> = { domain: d };
      if (testClient.trim()) params.client_ip = testClient.trim();
      const res = await axios.get("/api/test-domain", { params });
      setTestResult(res.data as DomainTestResult);
    } catch (e: unknown) {
      setTestError(
        (e as { response?: { data?: string } })?.response?.data ??
          "Request failed",
      );
    } finally {
      setTestLoading(false);
    }
  };

  const openDNSModal = async () => {
    const ua = navigator.userAgent.toLowerCase();
    const r = /iphone|ipad|ipod/.test(ua)
      ? "ios"
      : /android/.test(ua)
        ? "android"
        : /mac/.test(ua)
          ? "mac"
          : /linux/.test(ua)
            ? "linux"
            : /win/.test(ua)
              ? "windows"
              : "other";
    setModalInitialOS(r);
    try {
      const ipRes = await axios.get("/api/current-ip");
      setModalIP((ipRes.data as { ip: string }).ip ?? "");
    } catch {
      setModalIP("");
    }
    setShowDNSModal(true);
  };

  // ── shared class helpers ─────────────────────────────────────────────────

  const cardCls =
    "bg-surface-1 text-text rounded-[10px] border border-border shadow-[0_2px_8px_rgba(0,0,0,0.3)] p-[18px_20px]";
  const cardHeaderCls =
    "flex items-center justify-between mb-4 pb-3 border-b border-border-mid";
  const cardTitleCls =
    "flex items-center gap-2 text-[15px] font-bold text-text";
  const fieldLabelCls =
    "block text-[11px] font-semibold text-text-faint mb-1 uppercase tracking-[0.04em]";
  const inputCls =
    "w-full px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border";
  const textareaCls = `${inputCls} font-mono resize-y`;
  const btnPrimaryCls =
    "px-3.5 py-[9px] text-white border-none rounded-md cursor-pointer text-[13px] font-semibold w-full box-border transition-opacity";
  const btnDangerCls = `${btnPrimaryCls} bg-danger`;

  const SaveChip: React.FC<{ status: string }> = ({ status }) => {
    if (status === "saving")
      return (
        <span className="text-[11px] font-semibold px-2 py-0.5 rounded-full bg-[#aaa]/10 text-[#aaa] border border-[#aaa]/20">
          Saving…
        </span>
      );
    if (status === "saved")
      return (
        <span
          className="text-[11px] font-semibold px-2 py-0.5 rounded-full border"
          style={{
            background: `${ACCENT}22`,
            color: ACCENT,
            borderColor: `${ACCENT}44`,
          }}
        >
          ✓ Saved
        </span>
      );
    if (status === "error")
      return (
        <span className="text-[11px] font-semibold px-2 py-0.5 rounded-full bg-danger/10 text-danger border border-danger/20">
          ✗ Failed
        </span>
      );
    return null;
  };

  const UserChip: React.FC<{ label: string }> = ({ label }) => (
    <span
      className="text-[11px] font-semibold px-2 py-0.5 rounded-full border"
      style={{
        background: `${ACCENT}22`,
        color: ACCENT,
        borderColor: `${ACCENT}44`,
      }}
    >
      {label}
    </span>
  );

  return (
    <>
      {showDNSModal && (
        <DNSModal
          ip={modalIP}
          initialOS={modalInitialOS}
          onClose={() => setShowDNSModal(false)}
        />
      )}

      <div className={isMobile ? "p-3" : "p-5"}>
        {/* page title */}
        <div className="flex items-center gap-2 mb-4">
          <FaCog style={{ color: ACCENT, fontSize: 16 }} />
          <span className="text-[18px] font-bold text-text">Settings</span>
        </div>

        <div
          className={`grid gap-3.5 items-start ${isMobile ? "grid-cols-1" : "grid-cols-2"}`}
        >
          {/* ── Left column ── */}
          <div className="flex flex-col gap-3.5">
            {/* ── DNS Configuration ── */}
            <div className={cardCls}>
              <div className={cardHeaderCls}>
                <div className={cardTitleCls}>
                  <FaServer style={{ color: ACCENT }} />
                  DNS Configuration
                </div>
              </div>

              <div className="mb-3">
                <div className="flex items-center gap-2 mb-1">
                  <label className={fieldLabelCls}>Upstream DNS Servers</label>
                  <SaveChip status={upstreamSaveStatus} />
                </div>
                <textarea
                  value={upstream}
                  onChange={(e) => handleUpstreamChange(e.target.value)}
                  placeholder={"8.8.8.8\n1.1.1.1\n208.67.222.222"}
                  rows={4}
                  className={textareaCls}
                />
                <div className="flex flex-wrap gap-1.25 mt-1.75">
                  {upstreamPresets.map((r) => (
                    <button
                      key={r}
                      type="button"
                      onClick={() => {
                        handleUpstreamChange(
                          upstream + (upstream.trim() ? "\n" : "") + r,
                        );
                      }}
                      className="px-2 py-0.75 bg-border-dim text-[#bbb] border border-[#333] rounded cursor-pointer text-[11px] font-mono"
                    >
                      {r}
                    </button>
                  ))}
                </div>
              </div>

              {/* Custom Filtering Rules */}
              <div className="border-t border-border pt-3 mt-1">
                <div className="flex items-center gap-1.25 mb-1.5">
                  <label className="inline-flex items-center gap-1.25 text-[11px] font-semibold text-text-faint uppercase tracking-[0.04em]">
                    <FaFilter style={{ color: ACCENT, fontSize: 11 }} />
                    Custom Filtering Rules
                  </label>
                  <SaveChip status={rulesSaveStatus} />
                </div>
                <textarea
                  value={customRules}
                  onChange={(e) => handleRulesChange(e.target.value)}
                  placeholder={
                    "||example.org^\n@@||safe.org^\n127.0.0.1 ads.example.com"
                  }
                  rows={5}
                  className={textareaCls}
                />
                <RulesErrors rules={customRules} />
                <div className="text-[11px] text-text-ghost mt-1 leading-relaxed">
                  <code style={{ color: ACCENT }}>||domain^</code> block
                  &nbsp;·&nbsp;
                  <code style={{ color: ACCENT }}>@@||domain^</code> allow
                  &nbsp;·&nbsp;
                  <code style={{ color: ACCENT }}>127.0.0.1 domain</code>{" "}
                  redirect &nbsp;·&nbsp;
                  <code style={{ color: ACCENT }}># comment</code>
                </div>
              </div>
            </div>

            {/* ── DNS Blocklist ── */}
            <div className={cardCls}>
              <div className={cardHeaderCls}>
                <div className={cardTitleCls}>
                  <FaCog style={{ color: ACCENT }} />
                  DNS Blocklist
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-[11px] font-semibold px-2 py-0.5 rounded-full bg-[#aaa]/10 text-[#aaa] border border-[#aaa]/20">
                    {blocklist.length} domains
                  </span>
                  <button
                    onClick={reloadSources}
                    disabled={reloadLoading}
                    title="Reload lists"
                    className="flex items-center justify-center w-6 h-6 p-0 rounded shrink-0 cursor-pointer text-[11px] transition-[color,border-color,background] duration-150 disabled:cursor-not-allowed"
                    style={{
                      background: reloadLoading ? "#111" : "#1a211a",
                      border: `1px solid ${reloadLoading ? "#333" : ACCENT}`,
                      color: reloadLoading ? "#555" : ACCENT,
                    }}
                  >
                    <FaSyncAlt
                      style={{
                        animation: reloadLoading
                          ? "guardian-spin 0.9s linear infinite"
                          : "none",
                      }}
                    />
                  </button>
                  <ToggleSwitch
                    checked={blocklistEnabled}
                    onChange={toggleBlocklist}
                    label={blocklistEnabled ? "On" : "Off"}
                  />
                </div>
              </div>

              {/* Predefined lists */}
              <div className="mb-2.5">
                <label className={fieldLabelCls}>Predefined Blocklists</label>
                <div className="max-h-55 overflow-y-auto border border-[#252525] rounded-[5px] bg-surface-2 mt-1">
                  {predefined.map((item) => {
                    const active = activeSources.has(item.url);
                    const busy = sourceLoading === item.url;
                    return (
                      <label
                        key={item.name}
                        className="flex items-center gap-2.25 px-2.75 py-1.75 border-b border-surface-1 text-[12px] transition-[background] duration-150"
                        style={{
                          cursor: busy ? "wait" : "pointer",
                          background: active ? "#1a211a" : "transparent",
                        }}
                      >
                        <input
                          type="checkbox"
                          checked={active}
                          disabled={busy}
                          onChange={() => togglePredefined(item)}
                          className="shrink-0"
                          style={{ accentColor: ACCENT }}
                        />
                        <span
                          className="flex-1 transition-[color] duration-150"
                          style={{ color: active ? "#c8d4c6" : "#777" }}
                        >
                          {item.name}
                        </span>
                        {busy && (
                          <span className="text-[10px] text-text-ghost">
                            {active ? "Removing…" : "Loading…"}
                          </span>
                        )}
                        {active && !busy && (
                          <span
                            className="text-[10px] font-bold px-1.5 py-px rounded-full border"
                            style={{
                              background: `${ACCENT}22`,
                              color: ACCENT,
                              borderColor: `${ACCENT}44`,
                            }}
                          >
                            Active
                          </span>
                        )}
                      </label>
                    );
                  })}
                </div>
              </div>

              {/* Custom URLs */}
              <div className="mb-2.5">
                <div className="flex items-center gap-2 mb-1">
                  <label className={fieldLabelCls}>Custom Blocklist URLs</label>
                  <SaveChip status={urlsSaveStatus} />
                </div>
                <textarea
                  value={customURLs}
                  onChange={(e) => handleURLsChange(e.target.value)}
                  placeholder={"https://example.com/blocklist.txt"}
                  rows={3}
                  className={textareaCls}
                />
                <div className="text-[11px] text-text-dead mt-0.75">
                  One URL per line — saved automatically
                </div>
              </div>

              <button
                onClick={clearBlocklist}
                disabled={loading}
                className={`${btnDangerCls} ${loading ? "opacity-60" : ""}`}
              >
                {loading ? "Clearing…" : "Clear Blocklist"}
              </button>
            </div>

            {/* ── Query Log Retention ── */}
            <div className={cardCls}>
              <div className={cardHeaderCls}>
                <div className={cardTitleCls}>
                  <span style={{ color: ACCENT, fontSize: 13 }}>🗄</span>
                  Query Log Retention
                </div>
                <div className="flex items-center gap-2">
                  <SaveChip status={retentionStatus} />
                </div>
              </div>
              <div className="text-[12px] text-text-faint mb-2.5 leading-relaxed">
                Rows older than the window are pruned daily. Hard cap:
                500&thinsp;000 rows.
              </div>
              <div className="flex items-center gap-2">
                <input
                  type="number"
                  min={1}
                  max={3650}
                  value={retentionInput}
                  onChange={(e) => handleRetentionChange(e.target.value)}
                  className="w-20 px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border"
                />
                <span className="text-[12px] text-text-muted">days</span>
              </div>
            </div>
          </div>

          {/* ── Right column ── */}
          <div className="flex flex-col gap-3.5">
            {/* ── Test a Domain ── */}
            <DomainTestPanel
              isMobile={isMobile}
              testDomain={testDomain}
              setTestDomain={setTestDomain}
              testClient={testClient}
              setTestClient={setTestClient}
              testLoading={testLoading}
              testResult={testResult}
              testError={testError}
              onRun={runDomainTest}
            />

            {/* ── User Management ── */}
            <div className={cardCls}>
              <div className={cardHeaderCls}>
                <div className={cardTitleCls}>
                  <FaUser style={{ color: ACCENT }} />
                  User Management
                </div>
                <UserChip label={user ?? ""} />
              </div>
              <div className="flex gap-2.5 items-end">
                <div className="flex-1">
                  <label className={fieldLabelCls}>New Password</label>
                  <input
                    type="password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    placeholder="Enter new password"
                    className={inputCls}
                    onKeyDown={(e) => e.key === "Enter" && changePassword()}
                  />
                </div>
                <button
                  onClick={changePassword}
                  disabled={!newPassword}
                  className="px-3.5 py-2 text-white border-none rounded-md cursor-pointer text-[13px] font-semibold shrink-0 disabled:opacity-50 transition-opacity"
                  style={{ background: ACCENT, width: "auto" }}
                >
                  Change
                </button>
                {pwChanged && <UserChip label="✓ Updated" />}
              </div>
            </div>

            {/* ── DNS Setup Instructions ── */}
            <div className={cardCls}>
              <div className={cardHeaderCls}>
                <div className={cardTitleCls}>
                  <FaServer style={{ color: ACCENT }} />
                  DNS Setup Instructions
                </div>
              </div>
              <p className="text-[12px] text-text-ghost mt-0 mb-0">
                Need to point a device at this server?{" "}
                <button
                  onClick={openDNSModal}
                  className="bg-transparent border-none p-0 cursor-pointer text-[length:inherit] underline underline-offset-2"
                  style={{ color: ACCENT }}
                >
                  Re-open the setup guide →
                </button>
              </p>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};

export default SettingsPage;
