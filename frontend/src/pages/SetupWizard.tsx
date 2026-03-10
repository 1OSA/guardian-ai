import React, { useState } from "react";
import axios from "axios";
import { buildDNSInstructions } from "../lib/utils";

type CodeBlockProps = {
  text: string;
  id: string;
  copied: string | null;
  onCopy: (text: string, id: string) => void;
};

const CodeBlock: React.FC<CodeBlockProps> = ({ text, id, copied, onCopy }) => (
  <div className="relative bg-[#0d0d0d] border border-border-mid rounded-md mb-2">
    <pre className="m-0 px-3 pt-2.5 pb-2.5 pr-12 font-mono text-[12px] text-[#c8d4c6] whitespace-pre-wrap break-all">
      {text}
    </pre>
    <button
      onClick={() => onCopy(text, id)}
      title="Copy"
      className={`absolute top-1.5 right-1.5 border rounded text-[10px] px-1.75 py-0.5 cursor-pointer ${
        copied === id
          ? "bg-accent-dim border-accent text-accent"
          : "bg-surface-1 border-[#333] text-text-ghost"
      }`}
    >
      {copied === id ? "✓" : "Copy"}
    </button>
  </div>
);

const OSInstructionTabs: React.FC<{
  ip: string;
  defaultOS: string;
  accent: string;
  copied: string | null;
  onCopy: (text: string, id: string) => void;
}> = ({ ip, defaultOS, accent, copied, onCopy }) => {
  const instructions = buildDNSInstructions(ip);
  const osKeys = Object.keys(instructions);
  const [activeTab, setActiveTab] = useState<string>(defaultOS);
  const active = instructions[activeTab] ?? instructions["other"];

  return (
    <>
      <div className="flex flex-wrap gap-1 mb-3.5">
        {osKeys.map((key) => (
          <button
            key={key}
            onClick={() => setActiveTab(key)}
            className="px-3 py-1 rounded-full border text-[12px] font-semibold cursor-pointer"
            style={{
              border: `1px solid ${activeTab === key ? accent : "#333"}`,
              background: activeTab === key ? "#1a211a" : "transparent",
              color: activeTab === key ? accent : "#666",
            }}
          >
            {instructions[key].label}
          </button>
        ))}
      </div>

      <div className="bg-surface-2 border border-border-mid rounded-lg p-3.5 mb-5 text-[12px] text-[#aaa] leading-[1.7]">
        {active.steps.map((s, i) =>
          s.code !== undefined ? (
            <CodeBlock
              key={i}
              id={`${activeTab}-${i}`}
              text={s.code}
              copied={copied}
              onCopy={onCopy}
            />
          ) : (
            <p key={i} className={i === 0 ? "mt-0" : undefined}>
              {s.text}
            </p>
          ),
        )}
        {active.note && <p className="mb-0 text-text-ghost">{active.note}</p>}
      </div>
    </>
  );
};

const SetupWizard: React.FC<{
  onComplete: (username: string, password: string) => void;
}> = ({ onComplete }) => {
  const [step, setStep] = useState(1);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [currentIP, setCurrentIP] = useState("");
  const [copied, setCopied] = useState<string | null>(null);

  const ua = navigator.userAgent.toLowerCase();
  const detectedOS: "windows" | "mac" | "linux" | "android" | "ios" | "other" =
    /iphone|ipad|ipod/.test(ua)
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

  const copyToClipboard = (text: string, key: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(key);
      setTimeout(() => setCopied(null), 2000);
    });
  };

  const accent = "#798777";

  const nextStep = async () => {
    if (step === 1) {
      if (!username || !password || password !== confirmPassword) {
        setError("Please fill all fields and ensure passwords match.");
        return;
      }
      setLoading(true);
      try {
        await axios.post("/api/setup", { username, password });
        const ipRes = await axios.get("/api/current-ip");
        setCurrentIP(ipRes.data.ip);
        setStep(2);
        setError(null);
      } catch (err: unknown) {
        setError(
          (err as { response?: { data?: string } })?.response?.data ||
            "Setup failed",
        );
      } finally {
        setLoading(false);
      }
    } else if (step === 2) {
      onComplete(username, password);
    }
  };

  return (
    <div className="min-h-screen bg-surface flex items-center justify-center p-5">
      <div className="bg-surface-1 p-10 rounded-xl shadow-lg max-w-125 w-full box-border text-white">
        {step === 1 && (
          <>
            <h2 className="mt-0 mb-2 text-xl font-bold">
              Welcome to Guardian AI DNS
            </h2>
            <p className="text-[#aaa] mb-5">Let's set up your admin account.</p>
            {error && <div className="text-red-400 mb-4 text-sm">{error}</div>}
            <div className="mb-4">
              <label className="block mb-1.5 text-sm text-text">Username</label>
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full box-border px-2 py-2 border border-text-ghost rounded bg-surface text-white outline-none"
              />
            </div>
            <div className="mb-4">
              <label className="block mb-1.5 text-sm text-text">Password</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full box-border px-2 py-2 border border-text-ghost rounded bg-surface text-white outline-none"
              />
            </div>
            <div className="mb-4">
              <label className="block mb-1.5 text-sm text-text">
                Confirm Password
              </label>
              <input
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                className="w-full box-border px-2 py-2 border border-text-ghost rounded bg-surface text-white outline-none"
              />
            </div>
            <button
              onClick={nextStep}
              disabled={loading}
              className="w-full box-border py-2.5 px-5 bg-accent text-white border-none rounded cursor-pointer text-base font-semibold transition-opacity hover:opacity-80 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? "Setting up…" : "Next"}
            </button>
          </>
        )}

        {step === 2 && (
          <>
            <h2 className="mt-0 mb-2 text-xl font-bold">Setup Complete!</h2>
            <p className="text-[#aaa] mb-5">
              Your DNS server is running at{" "}
              <code
                className="bg-[#0d0d0d] px-1.5 py-0.5 rounded font-mono"
                style={{ color: accent }}
              >
                {currentIP}
              </code>
              . Point your devices or router at this address.
            </p>

            <OSInstructionTabs
              ip={currentIP}
              defaultOS={detectedOS}
              accent={accent}
              copied={copied}
              onCopy={copyToClipboard}
            />

            <button
              onClick={nextStep}
              className="w-full box-border py-2.5 px-5 bg-accent text-white border-none rounded cursor-pointer text-base font-semibold transition-opacity hover:opacity-80"
            >
              Go to Dashboard
            </button>
          </>
        )}
      </div>
    </div>
  );
};

export default SetupWizard;
