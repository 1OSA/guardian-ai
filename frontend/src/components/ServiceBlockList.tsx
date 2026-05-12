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
              border: `1px solid ${catFilter === cat ? ACCENT : "var(--color-border-mid)"}`,
              background:
                catFilter === cat ? "var(--color-accent-dim)" : "transparent",
              color: catFilter === cat ? ACCENT : "var(--color-text-faint)",
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
            border: `1px solid ${allFilteredBlocked ? "var(--color-danger-border)" : "var(--color-success-border)"}`,
            background: allFilteredBlocked
              ? "var(--color-danger-dim)"
              : "var(--color-success-dim)",
            color: allFilteredBlocked
              ? "var(--color-danger)"
              : "var(--color-accent)",
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
        const isScoped = scope === "client" || scope === "group";
        const scopeLabel = scope === "group" ? "group" : "client";
        // Treat any non-global source as an override for scoped (client/group) views
        const hasOverride =
          isScoped && !!sched.source && sched.source !== "global";
        const isInheriting = isScoped && !hasOverride;
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
              border: `1px solid ${sched.enabled ? "var(--color-danger-border)" : "var(--color-border)"}`,
              background: sched.enabled
                ? "var(--color-danger-dim)"
                : "var(--color-surface-3)",
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
                  <span className="text-[10px] px-1.5 py-px rounded-full bg-info-dim text-info border border-info-border">
                    {svc.category}
                  </span>
                  {isInheriting && (
                    <span
                      title={`No ${scopeLabel} override — inheriting from global schedule.`}
                      className="text-[10px] font-semibold px-1.5 py-px rounded-[3px] bg-surface-1 text-text-ghost border border-border-mid whitespace-nowrap"
                    >
                      Inherited
                    </span>
                  )}
                  {hasOverride && (
                    <span
                      title={`This ${scopeLabel} has a custom override for this service.`}
                      className="text-[10px] font-semibold px-1.5 py-px rounded-[3px] bg-accent-dim whitespace-nowrap border border-success-border"
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
                  background: isExpanded
                    ? "var(--color-border)"
                    : "transparent",
                  border: `1px solid ${hasSchedule && sched.enabled ? "var(--color-warn-border)" : "var(--color-border-mid)"}`,
                  color:
                    hasSchedule && sched.enabled
                      ? "var(--color-warn)"
                      : "var(--color-text-ghost)",
                }}
              >
                <FaClock />
              </button>

              {/* reset-to-global button */}
              {hasOverride && onReset && (
                <button
                  onClick={() => onReset(svc.id)}
                  disabled={isSaving}
                  title={`Remove ${scopeLabel} override — revert to inheriting global schedule`}
                  className="flex items-center gap-0.75 px-2 py-1.25 rounded-md border border-info-border bg-transparent text-text-ghost text-[10px] font-semibold shrink-0 whitespace-nowrap cursor-pointer disabled:cursor-wait"
                >
                  ↩ Inherit
                </button>
              )}

              {/* block toggle */}
              <button
                onClick={() => {
                  // Global scope: simple toggle
                  if (!isScoped) {
                    onSave(svc.id, { enabled: !sched.enabled });
                    return;
                  }

                  // Scoped (client/group): cycle through states
                  // - If inheriting: create an override -> Blocked (enabled=true)
                  // - If has override and blocked: switch to Allowed (explicit unblocked)
                  // - If has override and allowed: remove override (inherit)
                  if (isInheriting) {
                    // Create an override with the opposite of the effective (global) value.
                    // If the service is blocked globally, this will create an explicit unblocked override.
                    onSave(svc.id, { enabled: !effectiveEnabled });
                  } else {
                    if (sched.enabled) {
                      // Currently override is Blocked -> switch to Allowed (explicit unblocked)
                      onSave(svc.id, { enabled: false });
                    } else {
                      // Currently override is Allowed -> remove override (inherit)
                      if (onReset) onReset(svc.id);
                    }
                  }
                }}
                disabled={isSaving}
                title={
                  isScoped
                    ? isInheriting
                      ? effectiveEnabled
                        ? `Inheriting a global block — click to create a ${scopeLabel} override (Blocked). Click again to switch to Allowed.`
                        : `Inheriting a global allow — click to create a ${scopeLabel} override (Blocked).`
                      : sched.enabled
                        ? `${scopeLabel} override: Blocked. Click to switch to Allowed (explicitly allow for this ${scopeLabel}).`
                        : `${scopeLabel} override: Allowed (explicitly allowed). Click to remove override and revert to Inherit.`
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
                          ? "var(--color-danger-border)"
                          : "var(--color-border-mid)"
                        : sched.enabled
                          ? "var(--color-danger-border)"
                          : "var(--color-success-border)"
                      : sched.enabled
                        ? "var(--color-danger-border)"
                        : "var(--color-success-border)"
                  }`,
                  background:
                    scope === "client"
                      ? isInheriting
                        ? effectiveEnabled
                          ? "var(--color-danger-dim)"
                          : "var(--color-surface-2)"
                        : sched.enabled
                          ? "var(--color-danger-dim)"
                          : "var(--color-success-dim)"
                      : sched.enabled
                        ? "var(--color-danger-dim)"
                        : "var(--color-success-dim)",
                  color:
                    scope === "client"
                      ? isInheriting
                        ? effectiveEnabled
                          ? "var(--color-danger)"
                          : "var(--color-text-dead)"
                        : sched.enabled
                          ? "var(--color-danger)"
                          : "var(--color-accent)"
                      : sched.enabled
                        ? "var(--color-danger)"
                        : "var(--color-accent)",
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
                            border: `1px solid ${active ? ACCENT : "var(--color-border-mid)"}`,
                            background: active
                              ? "var(--color-accent-dim)"
                              : "var(--color-surface-3)",
                            color: active ? ACCENT : "var(--color-text-ghost)",
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
                      style={{ colorScheme: "light dark" }}
                    />
                    <span className="text-text-dead text-[12px]">to</span>
                    <input
                      type="time"
                      value={sched.time_end}
                      onChange={(e) =>
                        onSave(svc.id, { time_end: e.target.value })
                      }
                      className="px-1.75 py-1.25 bg-surface-3 border border-border-mid rounded-md text-text text-[12px]"
                      style={{ colorScheme: "light dark" }}
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
