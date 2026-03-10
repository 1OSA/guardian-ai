import React, { useState } from "react";
import { FaServer } from "react-icons/fa";
import { ACCENT } from "../lib/constants";
import { buildDNSInstructions } from "../lib/utils";

interface DNSModalProps {
  ip: string;
  initialOS?: string;
  onClose: () => void;
}

const DNSModal: React.FC<DNSModalProps> = ({
  ip,
  initialOS = "windows",
  onClose,
}) => {
  const [os, setOs] = useState(initialOS);
  const [copied, setCopied] = useState<string | null>(null);

  const instructions = buildDNSInstructions(ip);
  const osKeys = Object.keys(instructions);
  const active = instructions[os] ?? instructions["other"];

  return (
    <div
      className="fixed inset-0 bg-black/70 flex items-center justify-center z-1000 p-5"
      onClick={onClose}
    >
      <div
        className="bg-surface-1 border border-border-mid rounded-xl p-7 max-w-140 w-full box-border shadow-[0_8px_32px_rgba(0,0,0,0.6)] max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        {/* header */}
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <FaServer style={{ color: ACCENT, fontSize: 15 }} />
            <span className="text-[16px] font-bold text-text">
              DNS Setup Instructions
            </span>
          </div>
          <button
            onClick={onClose}
            className="bg-transparent border-none text-text-ghost cursor-pointer text-xl leading-none px-1 py-0"
          >
            ✕
          </button>
        </div>

        {/* subtitle */}
        <p className="text-[13px] text-[#aaa] mt-0 mb-4">
          Point your device or router's DNS at{" "}
          <code
            className="bg-[#0d0d0d] px-1.5 py-0.5 rounded font-mono"
            style={{ color: ACCENT }}
          >
            {ip || "your-server-ip"}
          </code>
        </p>

        {/* OS tabs */}
        <div className="flex flex-wrap gap-1 mb-3.5">
          {osKeys.map((key) => (
            <button
              key={key}
              onClick={() => setOs(key)}
              className="px-3 py-1 rounded-full border text-[12px] font-semibold cursor-pointer"
              style={{
                border: `1px solid ${os === key ? ACCENT : "#333"}`,
                background: os === key ? "#1a211a" : "transparent",
                color: os === key ? ACCENT : "#666",
              }}
            >
              {instructions[key].label}
            </button>
          ))}
        </div>

        {/* steps */}
        <div className="bg-surface-2 border border-border-mid rounded-lg p-3.5 text-[12px] text-[#aaa] leading-[1.7]">
          {active.steps.map((s, i) =>
            s.code !== undefined ? (
              <div
                key={i}
                className="relative bg-[#0d0d0d] border border-border-mid rounded-md mb-2"
              >
                <pre className="m-0 px-3 pt-2.5 pb-2.5 pr-11 font-mono text-[12px] text-[#c8d4c6] whitespace-pre-wrap break-all">
                  {s.code}
                </pre>
                <button
                  onClick={() =>
                    navigator.clipboard.writeText(s.code!).then(() => {
                      setCopied(`${os}-${i}`);
                      setTimeout(() => setCopied(null), 2000);
                    })
                  }
                  className="absolute top-1.5 right-1.5 border rounded text-[10px] px-1.75 py-0.5 cursor-pointer"
                  style={{
                    background: copied === `${os}-${i}` ? "#1a211a" : "#1a1a1a",
                    border: `1px solid ${copied === `${os}-${i}` ? ACCENT : "#333"}`,
                    color: copied === `${os}-${i}` ? ACCENT : "#555",
                  }}
                >
                  {copied === `${os}-${i}` ? "✓" : "Copy"}
                </button>
              </div>
            ) : (
              <p key={i} className={i === 0 ? "mt-0" : undefined}>
                {s.text}
              </p>
            ),
          )}
          {active.note && <p className="mb-0 text-text-ghost">{active.note}</p>}
        </div>

        {/* close button */}
        <button
          onClick={onClose}
          className="mt-4.5 w-full py-2.25 text-white border-none rounded-md cursor-pointer text-[13px] font-semibold transition-opacity hover:opacity-80"
          style={{ background: ACCENT }}
        >
          Close
        </button>
      </div>
    </div>
  );
};

export default DNSModal;
