import React, { useEffect, useState } from "react";
import axios from "axios";
import { FaBrain, FaThumbsUp, FaThumbsDown, FaDownload } from "react-icons/fa";
import { useMediaQuery } from "../lib/useMediaQuery";
import { ACCENT } from "../lib/constants";
import ToggleSwitch from "../components/ToggleSwitch";
import type { MLSettings } from "../lib/types";

const MLPage: React.FC = () => {
  const isMobile = useMediaQuery("(max-width: 768px)");

  const [settings, setSettings] = useState<MLSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [saveStatus, setSaveStatus] = useState<
    "idle" | "saving" | "saved" | "error"
  >("idle");
  const saveStatusTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );

  const [threshold, setThreshold] = useState(0.8);
  const [blockDGA, setBlockDGA] = useState(true);
  const [blockPhishing, setBlockPhishing] = useState(true);
  const [blockMalware, setBlockMalware] = useState(true);
  const [blockOther, setBlockOther] = useState(true);
  const [mlEnabled, setMlEnabled] = useState(true);

  const fetchSettings = async () => {
    setLoading(true);
    try {
      const res = await axios.get("/api/ml/settings");
      const d = res.data as MLSettings;
      setSettings(d);
      setThreshold(d.threshold);
      setBlockDGA(d.block_dga);
      setBlockPhishing(d.block_phishing);
      setBlockMalware(d.block_malware);
      setBlockOther(d.block_other);
      setMlEnabled(d.enabled ?? true);
    } catch {
      /* ignore */
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSettings();
  }, []);

  const toggleML = async (val: boolean) => {
    setMlEnabled(val);
    try {
      await axios.post("/api/ml/settings", { enabled: val });
    } catch {
      /* ignore */
    }
  };

  const save = React.useCallback(
    async (patch: {
      threshold?: number;
      block_dga?: boolean;
      block_phishing?: boolean;
      block_malware?: boolean;
      block_other?: boolean;
    }) => {
      setSaveStatus("saving");
      if (saveStatusTimerRef.current) clearTimeout(saveStatusTimerRef.current);
      try {
        await axios.post("/api/ml/settings", {
          threshold,
          block_dga: blockDGA,
          block_phishing: blockPhishing,
          block_malware: blockMalware,
          block_other: blockOther,
          ...patch,
        });
        setSaveStatus("saved");
        saveStatusTimerRef.current = setTimeout(
          () => setSaveStatus("idle"),
          2500,
        );
      } catch {
        setSaveStatus("error");
        saveStatusTimerRef.current = setTimeout(
          () => setSaveStatus("idle"),
          3000,
        );
      }
    },
    [threshold, blockDGA, blockPhishing, blockMalware, blockOther],
  );

  const cardCls = `bg-surface-1 text-text rounded-[10px] border border-border shadow-card mb-4 ${isMobile ? "p-3.5" : "p-5"}`;
  const cardHeaderCls =
    "flex items-center gap-2 mb-3.5 pb-3 border-b border-border";
  const statTileCls =
    "bg-surface-3 rounded-lg p-[10px_14px] border border-border-dim";

  return (
    <div className={isMobile ? "p-3" : "p-6"}>
      {/* header */}
      <div className="flex items-center justify-between gap-2 mb-5">
        <div className="flex items-center gap-2">
          <FaBrain style={{ color: ACCENT, fontSize: 16 }} />
          <span className="text-xl font-bold text-text">Threat Detection</span>
        </div>
        <ToggleSwitch
          checked={mlEnabled}
          onChange={toggleML}
          label={mlEnabled ? "Enabled" : "Disabled"}
        />
      </div>

      {loading ? (
        <div className="text-text-ghost p-6">Loading…</div>
      ) : (
        <>
          {/* status card */}
          <div className={cardCls}>
            <div className={cardHeaderCls}>
              <FaBrain style={{ color: ACCENT, fontSize: 13 }} />
              <span className="text-[14px] font-bold text-text">
                Model Status
              </span>
            </div>
            <div
              className={`grid gap-3 ${isMobile ? "grid-cols-2" : "grid-cols-4"}`}
            >
              {[
                {
                  label: "Connection",
                  value: settings?.ml_connected ? "Connected" : "Disconnected",
                  color: settings?.ml_connected ? ACCENT : "#c0392b",
                },
                {
                  label: "Cache entries",
                  value: `${settings?.cache_size ?? 0} / ${settings?.cache_max ?? 0}`,
                  color: "#aaa",
                },
                {
                  label: "Cache TTL",
                  value: `${settings?.cache_ttl_min ?? 0} min`,
                  color: "#aaa",
                },
                {
                  label: "Feedback collected",
                  value: `${settings?.feedback_total ?? 0}`,
                  color: "#aaa",
                },
              ].map((s) => (
                <div key={s.label} className={statTileCls}>
                  <div className="text-[11px] text-text-ghost mb-1 uppercase tracking-[0.05em]">
                    {s.label}
                  </div>
                  <div
                    className="text-[16px] font-bold"
                    style={{ color: s.color }}
                  >
                    {s.value}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* threshold card */}
          <div className={cardCls}>
            <div className={cardHeaderCls}>
              <span className="text-[14px] font-bold text-text">
                Confidence Threshold
              </span>
              <div className="ml-auto flex items-center gap-2.5">
                {saveStatus === "saving" && (
                  <span className="text-[11px] text-text-ghost">Saving…</span>
                )}
                {saveStatus === "saved" && (
                  <span className="text-[11px]" style={{ color: ACCENT }}>
                    ✓ Saved
                  </span>
                )}
                {saveStatus === "error" && (
                  <span className="text-[11px] text-danger">
                    Failed to save
                  </span>
                )}
                <span
                  className="text-xl font-bold tabular-nums"
                  style={{ color: ACCENT }}
                >
                  {Math.round(threshold * 100)}%
                </span>
              </div>
            </div>
            <input
              type="range"
              min={10}
              max={99}
              value={Math.round(threshold * 100)}
              onChange={(e) => setThreshold(Number(e.target.value) / 100)}
              onMouseUp={(e) =>
                save({
                  threshold: Number((e.target as HTMLInputElement).value) / 100,
                })
              }
              onTouchEnd={(e) =>
                save({
                  threshold: Number((e.target as HTMLInputElement).value) / 100,
                })
              }
              className="w-full cursor-pointer"
              style={{ accentColor: ACCENT }}
            />
            <div className="flex justify-between mt-1.5 text-[11px] text-text-dead">
              <span>10% — aggressive (more blocks)</span>
              <span>99% — lenient (fewer blocks)</span>
            </div>
            <div className="mt-3 text-[12px] text-text-ghost leading-relaxed">
              A domain is blocked only when the model confidence is at or above
              this value. Current setting:{" "}
              <span className="text-[#aaa]">
                block if confidence ≥ {Math.round(threshold * 100)}%
              </span>
              .
            </div>
          </div>

          {/* category toggles card */}
          <div className={cardCls}>
            <div className="text-[14px] font-bold text-text mb-3.5 pb-3 border-b border-border">
              Category Filters
            </div>
            <div className="flex flex-col gap-2.5">
              {(
                [
                  {
                    key: "dga",
                    label: "DGA",
                    desc: "Domain Generation Algorithm — randomised malware C2 domains",
                    val: blockDGA,
                    set: setBlockDGA,
                  },
                  {
                    key: "phishing",
                    label: "Phishing",
                    desc: "Lookalike and credential-harvesting domains",
                    val: blockPhishing,
                    set: setBlockPhishing,
                  },
                  {
                    key: "malware",
                    label: "Malware",
                    desc: "Known malware distribution and C2 infrastructure",
                    val: blockMalware,
                    set: setBlockMalware,
                  },
                  {
                    key: "other",
                    label: "Other",
                    desc: "Any malicious category not matched above",
                    val: blockOther,
                    set: setBlockOther,
                  },
                ] as {
                  key: string;
                  label: string;
                  desc: string;
                  val: boolean;
                  set: (v: boolean) => void;
                }[]
              ).map((cat) => (
                <div
                  key={cat.key}
                  className="flex items-center justify-between px-3.5 py-3 bg-surface-2 rounded-lg border border-border-dim"
                >
                  <div>
                    <div className="text-[14px] font-semibold text-text">
                      {cat.label}
                    </div>
                    <div className="text-[12px] text-text-ghost mt-0.5">
                      {cat.desc}
                    </div>
                  </div>
                  <ToggleSwitch
                    checked={cat.val}
                    onChange={(v) => {
                      cat.set(v);
                      save({
                        [cat.key === "dga"
                          ? "block_dga"
                          : cat.key === "phishing"
                            ? "block_phishing"
                            : cat.key === "malware"
                              ? "block_malware"
                              : "block_other"]: v,
                      });
                    }}
                  />
                </div>
              ))}
            </div>
          </div>

          {/* feedback card */}
          <div className={cardCls}>
            <div className="text-[14px] font-bold text-text mb-3.5 pb-3 border-b border-border">
              Feedback Dataset
            </div>
            <div
              className={`grid gap-3 mb-4 ${isMobile ? "grid-cols-2" : "grid-cols-3"}`}
            >
              {[
                {
                  label: "Total feedback",
                  value: settings?.feedback_total ?? 0,
                  color: "#aaa",
                },
                {
                  label: "Marked safe",
                  value: settings?.feedback_safe ?? 0,
                  color: ACCENT,
                },
                {
                  label: "Confirmed malicious",
                  value: settings?.feedback_mal ?? 0,
                  color: "#c0392b",
                },
              ].map((s) => (
                <div key={s.label} className={statTileCls}>
                  <div className="text-[11px] text-text-ghost mb-1 uppercase tracking-[0.05em]">
                    {s.label}
                  </div>
                  <div
                    className="text-[22px] font-bold"
                    style={{ color: s.color }}
                  >
                    {s.value}
                  </div>
                </div>
              ))}
            </div>
            <div className="text-[12px] text-text-ghost mb-3.5 leading-relaxed">
              Feedback is collected when you click{" "}
              <FaThumbsUp className="inline align-middle" /> /{" "}
              <FaThumbsDown className="inline align-middle" /> on ML-blocked
              rows in the Query Log. Export this data to retrain the model
              offline.
            </div>
            <a
              href="/api/ml/feedback/export"
              className="inline-flex items-center gap-1.75 px-4 py-1.75 bg-surface-1 text-[#aaa] border border-[#333] rounded-md no-underline text-[13px] font-semibold"
            >
              <FaDownload className="text-[11px]" />
              Export feedback CSV
            </a>
          </div>
        </>
      )}
    </div>
  );
};

export default MLPage;
