import React from "react";
import { FaNetworkWired } from "react-icons/fa";
import { ACCENT } from "../lib/constants";
import RulesErrors from "./RulesErrors";
import ToggleSwitch from "./ToggleSwitch";

interface ClientAddFormProps {
  isMobile: boolean;
  formName: string;
  setFormName: (v: string) => void;
  formLabel: string;
  setFormLabel: (v: string) => void;
  formMembers: string;
  setFormMembers: (v: string) => void;
  formRules: string;
  setFormRules: (v: string) => void;
  formBlocked: boolean;
  setFormBlocked: (v: boolean) => void;
  saving: boolean;
  onSave: () => void;
  onCancel: () => void;
}

const ClientAddForm: React.FC<ClientAddFormProps> = ({
  isMobile,
  formName,
  setFormName,
  formLabel,
  setFormLabel,
  formMembers,
  setFormMembers,
  formRules,
  setFormRules,
  formBlocked,
  setFormBlocked,
  saving,
  onSave,
  onCancel,
}) => (
  <div className="bg-surface-2 border border-border-mid rounded-lg p-4 mb-4">
    {formMembers && formName === formMembers.trim() && (
      <div
        className="flex items-center gap-2 px-3 py-2 rounded-md mb-3 text-[12px] text-[#9ab899]"
        style={{ background: "#1a1f1a", border: `1px solid ${ACCENT}44` }}
      >
        <FaNetworkWired className="shrink-0 text-[11px]" />
        New client pre-filled from query log — edit the name below then save.
      </div>
    )}

    <div
      className={`grid gap-3 mb-3 ${isMobile ? "grid-cols-1" : "grid-cols-2"}`}
    >
      <div>
        <label className="block text-[11px] font-semibold text-text-faint mb-1 uppercase tracking-[0.04em]">
          Name *
        </label>
        <input
          value={formName}
          onChange={(e) => setFormName(e.target.value)}
          placeholder="e.g. Kids, Dad's laptop"
          className="w-full px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border"
          autoFocus
        />
      </div>
      <div>
        <label className="block text-[11px] font-semibold text-text-faint mb-1 uppercase tracking-[0.04em]">
          Description (optional)
        </label>
        <input
          value={formLabel}
          onChange={(e) => setFormLabel(e.target.value)}
          placeholder="e.g. Bedroom devices"
          className="w-full px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border"
        />
      </div>
    </div>

    <div className="mb-3">
      <label className="block text-[11px] font-semibold text-text-faint mb-1 uppercase tracking-[0.04em]">
        IPs / MACs (one per line or comma-separated)
      </label>
      <textarea
        value={formMembers}
        onChange={(e) => setFormMembers(e.target.value)}
        placeholder={"192.168.1.10\n192.168.1.11\naa:bb:cc:dd:ee:ff"}
        className="w-full px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border font-mono resize-y min-h-20"
      />
      <span className="text-[11px] text-text-ghost">
        MAC addresses (17 chars with colons) are auto-detected. IPs matched at
        query time.
      </span>
    </div>

    <div className="mb-3">
      <label className="block text-[11px] font-semibold text-text-faint mb-1 uppercase tracking-[0.04em]">
        Custom Filtering Rules
      </label>
      <textarea
        value={formRules}
        onChange={(e) => setFormRules(e.target.value)}
        placeholder={
          "! Block a domain\n||ads.example.com^\n\n! Allow a domain\n@@||safe.example.com^"
        }
        className="w-full px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border font-mono resize-y min-h-25"
      />
      <RulesErrors rules={formRules} />
      <span className="text-[11px] text-text-ghost">
        <code className="text-danger">||domain^</code> block{"  ·  "}
        <code className="text-accent">@@||domain^</code> allow
      </span>
    </div>

    <div className="mb-3">
      <ToggleSwitch
        checked={formBlocked}
        onChange={setFormBlocked}
        label="Block all DNS for this client"
      />
    </div>

    <div className="flex gap-2">
      <button
        onClick={onSave}
        disabled={saving || !formName.trim()}
        className="px-4 py-1.75 text-white border-none rounded-md text-[13px] font-semibold cursor-pointer disabled:opacity-50 disabled:cursor-wait"
        style={{ background: ACCENT }}
      >
        {saving ? "Saving…" : "Save"}
      </button>
      <button
        onClick={onCancel}
        className="px-4 py-1.75 bg-transparent text-text-muted border border-border-mid rounded-md cursor-pointer text-[13px]"
      >
        Cancel
      </button>
    </div>
  </div>
);

export default ClientAddForm;
