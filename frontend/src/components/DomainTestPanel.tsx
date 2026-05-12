import React from "react";
import { FaShieldAlt } from "react-icons/fa";
import { ACCENT } from "../lib/constants";

export type DomainTestResult = {
  domain: string;
  client_ip: string;
  blocked: boolean;
  reason: string;
  category: string;
  confidence: number;
  checks: string[];
};

type Props = {
  testDomain: string;
  setTestDomain: (v: string) => void;
  testClient: string;
  setTestClient: (v: string) => void;
  testLoading: boolean;
  testResult: DomainTestResult | null;
  testError: string | null;
  onRun: () => void;
  isMobile: boolean;
};

const DomainTestPanel: React.FC<Props> = ({
  testDomain,
  setTestDomain,
  testClient,
  setTestClient,
  testLoading,
  testResult,
  testError,
  onRun,
  isMobile,
}) => {
  return (
    <div
      className={`bg-surface-1 text-text rounded-[10px] border border-border shadow-card ${isMobile ? "p-3.5" : "p-[18px_20px]"}`}
    >
      {/* header */}
      <div className="flex items-center justify-between mb-4 pb-3 border-b border-border-mid">
        <div className="flex items-center gap-2 text-[15px] font-bold text-text">
          <FaShieldAlt style={{ color: ACCENT }} />
          Test a Domain
        </div>
      </div>

      <p className="text-[12px] text-text-ghost mt-0 mb-3.5 leading-relaxed">
        Dry-run the full block-decision pipeline for any domain without sending
        real DNS traffic. Optionally scope it to a specific client IP.
      </p>

      {/* inputs + button */}
      <div className="flex gap-2 flex-wrap mb-2.5">
        <input
          value={testDomain}
          onChange={(e) => setTestDomain(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && onRun()}
          placeholder="domain.com"
          className="flex-[2_1_180px] px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border"
        />
        <input
          value={testClient}
          onChange={(e) => setTestClient(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && onRun()}
          placeholder="Client IP (optional)"
          className="flex-[1_1_140px] px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border"
        />
        <button
          onClick={onRun}
          disabled={testLoading || !testDomain.trim()}
          className="px-4.5 py-2 text-white border-none rounded-md cursor-pointer text-[13px] font-semibold transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
          style={{ background: ACCENT }}
        >
          {testLoading ? "Testing…" : "Test"}
        </button>
      </div>

      {/* error */}
      {testError && (
        <div className="text-[12px] text-danger mb-2">{testError}</div>
      )}

      {/* result */}
      {testResult && (
        <div
          className="rounded-lg p-3 px-4"
          style={{
            background: testResult.blocked ? "#1c1010" : "#101c10",
            border: `1px solid ${testResult.blocked ? "#7a3333" : "#3a5a3a"}`,
          }}
        >
          {/* verdict row */}
          <div className="flex items-center gap-2.5 mb-2.5 flex-wrap">
            <span
              className="text-[13px] font-bold px-3 py-0.75 rounded-full border whitespace-nowrap"
              style={{
                background: testResult.blocked ? "#2a1414" : "#141a14",
                color: testResult.blocked ? "#c0392b" : ACCENT,
                border: `1px solid ${testResult.blocked ? "#7a3333" : "#3a5a3a"}`,
              }}
            >
              {testResult.blocked ? "⛔ BLOCKED" : "✓ ALLOWED"}
            </span>
            <code className="text-[12px] text-[#aaa]">{testResult.domain}</code>
            {testResult.client_ip && (
              <span className="text-[11px] text-text-ghost">
                for {testResult.client_ip}
              </span>
            )}
            {testResult.blocked && testResult.reason && (
              <span className="text-[11px] text-text-muted ml-auto font-mono">
                reason: {testResult.reason}
              </span>
            )}
          </div>

          {/* category + confidence */}
          {testResult.category && testResult.category !== "" && (
            <div className="text-[11px] text-text-ghost mb-2">
              Category:{" "}
              <span className="text-[#aaa]">{testResult.category}</span>
              {testResult.confidence > 0 && (
                <>
                  {" · "}Confidence:{" "}
                  <span style={{ color: ACCENT }}>
                    {Math.round(testResult.confidence * 100)}%
                  </span>
                </>
              )}
            </div>
          )}

          {/* decision trace */}
          <div>
            <div className="text-[11px] text-text-dead mb-1.25 uppercase tracking-[0.05em]">
              Decision trace
            </div>
            {testResult.checks.map((c, i) => {
              const isMatch = c.includes("matched") || c.includes("blocked");
              const isSkip = c.includes("skipped") || c.includes("disabled");
              return (
                <div
                  key={i}
                  className="flex items-start gap-2 py-0.75 text-[11px] font-mono"
                  style={{
                    color: isMatch ? "#e07070" : isSkip ? "#444" : "#666",
                  }}
                >
                  <span className="text-[#333] shrink-0">{i + 1}.</span>
                  {c}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};

export default DomainTestPanel;
