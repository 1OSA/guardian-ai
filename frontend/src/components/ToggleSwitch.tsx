import React from "react";

const ToggleSwitch: React.FC<{
  checked: boolean;
  onChange: (v: boolean) => void;
  label?: string;
}> = ({ checked, onChange, label }) => (
  <label className="flex items-center gap-2.5 cursor-pointer select-none">
    <div
      onClick={() => onChange(!checked)}
      className={`relative w-10 h-5.5 rounded-full border shrink-0 cursor-pointer transition-[background,border] duration-200 ${
        checked ? "bg-accent border-accent" : "bg-[#3a3a3a] border-text-ghost"
      }`}
    >
      <div
        className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow-[0_1px_3px_rgba(0,0,0,0.4)] transition-[left] duration-200 ${
          checked ? "left-4.5" : "left-0.5"
        }`}
      />
    </div>
    {label && (
      <span
        className={`text-[13px] ${checked ? "text-[#c8d4c6]" : "text-text-muted"}`}
      >
        {label}
      </span>
    )}
  </label>
);

export default ToggleSwitch;
