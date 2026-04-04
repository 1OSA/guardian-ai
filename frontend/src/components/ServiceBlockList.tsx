import React, { useState } from "react";
import { FaLock, FaClock } from "react-icons/fa";
import { ACCENT, DAY_LABELS } from "../lib/constants";
import { getServiceIcon } from "./ServiceIcons";
import type {
  ServiceDef,
  ServiceSchedule,
  ServiceScheduleMap,
} from "../lib/types";

export type ServiceBlockListProps = {
  scope: "global" | "client" | "group";
  defs: ServiceDef[];
  /** In client/group scope this must be the merged schedule (merged=1 API param),
   *  so every entry carries a source field ("client" | "group" | "global"). */
  schedules: ServiceScheduleMap;
  saving: string | null;
  onSave: (svcId: string, patch: Partial<ServiceSchedule>) => void;
  /** Called when the user removes a client/group override row (reverts to inheriting global). */
  onReset?: (svcId: string) => void;
};

const ServiceBlockList: React.FC<ServiceBlockListProps> = ({
  scope,
  defs,
  schedules,
  saving,
  onSave,
  onReset,
}) => {
  const [expanded, setExpanded] = useState<string | null>(null);
  const [catFilter, setCatFilter] = useState<string>("All");

  const getSchedule = (svcId: string): ServiceSchedule =>
    schedules[svcId] ?? {
      enabled: false,
      days_of_week: "",
      time_start: "",
      time_end: "",
    };

  const toggleDay = (svcId: string, day: number) => {
    const sched = getSchedule(svcId);
    const current = sched.days_of_week
      ? sched.days_of_week.split(",").map(Number)
      : [];
    const next = current.includes(day)
      ? current.filter((d: number) => d !== day)
      : [...current, day].sort();
    onSave(svcId, { days_of_week: next.join(",") });
  };

  const categories = [
    "All",
    ...Array.from(new Set(defs.map((d) => d.category))),
  ];
  const filtered =
    catFilter === "All" ? defs : defs.filter((d) => d.category === catFilter);

  const allFilteredBlocked =
    filtered.length > 0 && filtered.every((svc) => getSchedule(svc.id).enabled);

  const toggleBlockAll = () => {
    const enable = !allFilteredBlocked;
    filtered.forEach((svc) => {
      if (getSchedule(svc.id).enabled !== enable) {
        onSave(svc.id, { enabled: enable });
      }
    });
  };

  const schedSummary = (sched: ServiceSchedule) => {
    if (!sched.enabled) return null;
    const parts: string[] = [];
    if (sched.days_of_week) {
      const days = sched.days_of_week.split(",").map(Number);
      parts.push(days.map((d: number) => DAY_LABELS[d]).join(" "));
    }
    if (sched.time_start && sched.time_end) {
      parts.push(`${sched.time_start}–${sched.time_end}`);
    }
    if (parts.length === 0)
      return <span className="text-[10px] text-danger">Always blocked</span>;
    return <span className="text-[10px] text-warn">{parts.join(" · ")}</span>;
  };

  return (
    <>
      {/* category filter pills + block-all button */}
      <div className="flex flex-wrap gap-1.5 mb-3 items-center">
        {categories.map((cat) => (
          <button
            key={cat}
            onClick={() => setCatFilter(cat)}
            className="px-2.5 py-0.75 rounded-full text-[11px] font-semibold cursor-pointer"
            style={{
              border: `1px solid ${catFilter === cat ? ACCENT : "#2a2a2a"}`,
              background: catFilter === cat ? "#1a211a" : "transparent",
              color: catFilter === cat ? ACCENT : "#666",
            }}
          >
            {cat}
          </button>
        ))}

        <div className="flex-1" />

        <button
          onClick={toggleBlockAll}
          title={
            allFilteredBlocked
              ? `Unblock all${catFilter !== "All" ? ` ${catFilter}` : ""}`
              : `Block all${catFilter !== "All" ? ` ${catFilter}` : ""}`
          }
          className="flex items-center gap-1.25 px-2.75 py-0.75 rounded-full text-[11px] font-bold cursor-pointer whitespace-nowrap"
          style={{
            border: `1px solid ${allFilteredBlocked ? "#7a3333" : "#2a3a2a"}`,
            background: allFilteredBlocked ? "#2a1414" : "#141a14",
            color: allFilteredBlocked ? "#c0392b" : "#798777",
          }}
        >
          <FaLock className="text-[9px]" />
          {allFilteredBlocked
            ? `Unblock all${catFilter !== "All" ? ` — ${catFilter}` : ""}`
            : `Block all${catFilter !== "All" ? ` — ${catFilter}` : ""}`}
        </button>
      </div>

      {/* service rows */}
      {filtered.map((svc) => {
        const sched = getSchedule(svc.id);
        const isExpanded = expanded === svc.id;
        const isSaving = saving === svc.id;
        // Treat any non-global source (e.g., 'client' or 'group') as an override for client scope
        const hasOverride =
          scope === "client" && !!sched.source && sched.source !== "global";
        const isInheriting = scope === "client" && !hasOverride;
        const effectiveEnabled = sched.enabled;
        const hasSchedule = !!(
          sched.days_of_week ||
          (sched.time_start && sched.time_end)
        );

        return (
          <div
            key={svc.id}
            className="rounded-lg mb-2 overflow-hidden transition-[border-color] duration-200"
            style={{
              border: `1px solid ${sched.enabled ? "#3a2a2a" : "#222"}`,
              background: sched.enabled ? "#1c1414" : "#0f0f0f",
            }}
          >
            {/* row */}
            <div className="flex items-center gap-2.5 px-3.5 py-2.5">
              <span className="text-[18px] shrink-0 leading-none flex items-center">
                {getServiceIcon(svc.id)}
              </span>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-1.5 flex-wrap">
                  <span className="text-[13px] font-semibold text-text">
                    {svc.name}
                  </span>
                  <span className="text-[10px] px-1.5 py-px rounded-full bg-[#1a1a2a] text-[#7a8fa0] border border-[#2a2a3a]">
                    {svc.category}
                  </span>
                  {isInheriting && (
                    <span
                      title="No client override — inheriting from global schedule."
                      className="text-[10px] font-semibold px-1.5 py-px rounded-[3px] bg-surface-1 text-text-ghost border border-border-mid whitespace-nowrap"
                    >
                      Inherited
                    </span>
                  )}
                  {hasOverride && (
                    <span
                      title="This client has a custom override for this service."
                      className="text-[10px] font-semibold px-1.5 py-px rounded-[3px] bg-accent-dim whitespace-nowrap border border-[#2a3a2a]"
                      style={{ color: ACCENT }}
                    >
                      Custom
                    </span>
                  )}
                  {schedSummary(sched)}
                </div>
                <div className="text-[11px] text-text-dead mt-px">
                  {svc.domains.slice(0, 3).join(", ")}
                  {svc.domains.length > 3 && ` +${svc.domains.length - 3} more`}
                </div>
              </div>

              {/* schedule button */}
              <button
                onClick={() => setExpanded(isExpanded ? null : svc.id)}
                title="Configure schedule"
                className="flex items-center justify-center w-6.5 h-6.5 p-0 rounded shrink-0 cursor-pointer text-[11px]"
                style={{
                  background: isExpanded ? "#222" : "transparent",
                  border: `1px solid ${hasSchedule && sched.enabled ? "#5a3a1a" : "#2a2a2a"}`,
                  color: hasSchedule && sched.enabled ? "#e67e22" : "#555",
                }}
              >
                <FaClock />
              </button>

              {/* reset-to-global button */}
              {hasOverride && onReset && (
                <button
                  onClick={() => onReset(svc.id)}
                  disabled={isSaving}
                  title="Remove client override — revert to inheriting global schedule"
                  className="flex items-center gap-0.75 px-2 py-1.25 rounded-md border border-[#2a2a3a] bg-transparent text-text-ghost text-[10px] font-semibold shrink-0 whitespace-nowrap cursor-pointer disabled:cursor-wait"
                >
                  ↩ Inherit
                </button>
              )}

              {/* block toggle */}
              <button
                onClick={() => {
                  // Global scope: simple toggle (anything that's not client/group)
                  if (!(scope === "client" || scope === "group")) {
                    onSave(svc.id, { enabled: !sched.enabled });
                    return;
                  }

                  // Client scope: cycle through states
                  // - If inheriting: create a client override -> Blocked (enabled=true)
                  // - If has override and client-blocked: switch to client-unblocked (enabled=false)
                  // - If has override and client-unblocked: remove override (inherit)
                  if (isInheriting) {
                    // Create an override with the opposite of the effective (global) value.
                    // If the service is blocked globally, this will create an explicit unblocked override.
                    onSave(svc.id, { enabled: !effectiveEnabled });
                  } else {
                    if (sched.enabled) {
                      // Currently client override is Blocked -> switch to Allowed (explicit unblocked)
                      onSave(svc.id, { enabled: false });
                    } else {
                      // Currently client override is Unblocked -> remove override (inherit)
                      if (onReset) onReset(svc.id);
                    }
                  }
                }}
                disabled={isSaving}
                title={
                  scope === "client"
                    ? isInheriting
                      ? effectiveEnabled
                        ? "Inheriting a global block — click to create a client override (Blocked). Click again to switch to Allowed."
                        : "Inheriting a global allow — click to create a client override (Blocked)."
                      : sched.enabled
                        ? "Client override: Blocked. Click to switch to Allowed (explicitly allow for this client)."
                        : "Client override: Allowed (explicitly allowed). Click to remove override and revert to Inherit."
                    : sched.enabled
                      ? "Click to unblock (global)"
                      : "Click to block (global)"
                }
                className="flex items-center gap-1 px-2.5 py-1.25 rounded-md text-[11px] font-bold shrink-0 cursor-pointer disabled:cursor-wait"
                style={{
                  border: `1px solid ${
                    // Highlight red when the effective or override is blocked
                    scope === "client"
                      ? isInheriting
                        ? effectiveEnabled
                          ? "#7a3333"
                          : "#2a2a2a"
                        : sched.enabled
                          ? "#7a3333"
                          : "#2a3a2a"
                      : sched.enabled
                        ? "#7a3333"
                        : "#2a3a2a"
                  }`,
                  background:
                    scope === "client"
                      ? isInheriting
                        ? effectiveEnabled
                          ? "#2a1414"
                          : "#111"
                        : sched.enabled
                          ? "#2a1414"
                          : "#141a14"
                      : sched.enabled
                        ? "#2a1414"
                        : "#141a14",
                  color:
                    scope === "client"
                      ? isInheriting
                        ? effectiveEnabled
                          ? "#c0392b"
                          : "#444"
                        : sched.enabled
                          ? "#c0392b"
                          : "#798777"
                      : sched.enabled
                        ? "#c0392b"
                        : "#798777",
                  opacity: isSaving ? 0.6 : 1,
                }}
              >
                <FaLock className="text-[9px]" />
                {scope === "client"
                  ? isInheriting
                    ? effectiveEnabled
                      ? "Blocked (global)"
                      : "Inherit"
                    : sched.enabled
                      ? "Blocked"
                      : "Allowed"
                  : sched.enabled
                    ? "Blocked"
                    : "Allowed"}
              </button>
            </div>

            {/* schedule editor */}
            {isExpanded && (
              <div className="px-3.5 pt-2.5 pb-3.5 border-t border-border-dim bg-surface-2">
                <div className="text-[11px] text-text-faint mb-2">
                  <FaClock className="inline mr-1 align-middle" />
                  Schedule — only block during selected days / time window.
                  Leave empty to block always.
                </div>

                {/* days */}
                <div className="mb-2.5">
                  <div className="text-[11px] font-semibold text-text-faint mb-1 uppercase tracking-[0.05em]">
                    Days of week
                  </div>
                  <div className="flex gap-1.25 flex-wrap">
                    {DAY_LABELS.map((label, idx) => {
                      const active = sched.days_of_week
                        ? sched.days_of_week
                            .split(",")
                            .map(Number)
                            .includes(idx)
                        : false;
                      return (
                        <button
                          key={idx}
                          onClick={() => toggleDay(svc.id, idx)}
                          className="w-8.5 h-8.5 rounded-md text-[11px] font-semibold cursor-pointer"
                          style={{
                            border: `1px solid ${active ? ACCENT : "#2a2a2a"}`,
                            background: active ? "#1a211a" : "#0f0f0f",
                            color: active ? ACCENT : "#555",
                          }}
                        >
                          {label}
                        </button>
                      );
                    })}
                    {sched.days_of_week && (
                      <button
                        onClick={() => onSave(svc.id, { days_of_week: "" })}
                        className="px-2 h-8.5 rounded-md border border-border-mid bg-transparent text-text-ghost cursor-pointer text-[10px]"
                      >
                        Clear
                      </button>
                    )}
                  </div>
                </div>

                {/* time range */}
                <div>
                  <div className="text-[11px] font-semibold text-text-faint mb-1 uppercase tracking-[0.05em]">
                    Time window (optional)
                  </div>
                  <div className="flex items-center gap-2 flex-wrap">
                    <input
                      type="time"
                      value={sched.time_start}
                      onChange={(e) =>
                        onSave(svc.id, { time_start: e.target.value })
                      }
                      className="px-1.75 py-1.25 bg-surface-3 border border-border-mid rounded-md text-text text-[12px]"
                      style={{ colorScheme: "dark" }}
                    />
                    <span className="text-text-dead text-[12px]">to</span>
                    <input
                      type="time"
                      value={sched.time_end}
                      onChange={(e) =>
                        onSave(svc.id, { time_end: e.target.value })
                      }
                      className="px-1.75 py-1.25 bg-surface-3 border border-border-mid rounded-md text-text text-[12px]"
                      style={{ colorScheme: "dark" }}
                    />
                    {(sched.time_start || sched.time_end) && (
                      <button
                        onClick={() =>
                          onSave(svc.id, { time_start: "", time_end: "" })
                        }
                        className="px-2 py-1.25 rounded-md border border-border-mid bg-transparent text-text-ghost cursor-pointer text-[10px]"
                      >
                        Clear
                      </button>
                    )}
                  </div>
                  {!sched.days_of_week &&
                    sched.time_start &&
                    sched.time_end && (
                      <p className="text-[10px] text-text-ghost mt-1 mb-0">
                        No days selected — time window applies every day.
                      </p>
                    )}
                </div>
              </div>
            )}
          </div>
        );
      })}
    </>
  );
};

export default ServiceBlockList;
