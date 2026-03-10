import React from "react";
import { validateRules } from "../lib/utils";

const RulesErrors: React.FC<{ rules: string }> = ({ rules }) => {
  const errs = validateRules(rules);
  if (errs.length === 0) return null;
  return (
    <div className="mt-1 px-2.5 py-1.5 bg-danger-dim border border-danger-border/30 rounded text-[11px] text-danger">
      {errs.map((e) => (
        <div key={e.line} className="mb-0.5">
          <span className="text-text-ghost mr-1.5">Line {e.line}:</span>
          <code className="text-[#e07070] mr-1.5">
            {e.text.length > 40 ? e.text.slice(0, 40) + "…" : e.text}
          </code>
          <span>{e.error}</span>
        </div>
      ))}
    </div>
  );
};

export default RulesErrors;
