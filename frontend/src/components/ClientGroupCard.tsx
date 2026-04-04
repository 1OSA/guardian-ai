import React from "react";
import {
  FaNetworkWired,
  FaPlus,
  FaTrash,
  FaEdit,
  FaCheck,
} from "react-icons/fa";
import { ACCENT } from "../lib/constants";
import RulesErrors from "./RulesErrors";
import ToggleSwitch from "./ToggleSwitch";
import ServiceBlockList from "./ServiceBlockList";
import type {
  ClientGroup,
  GroupMember,
  ServiceDef,
  ServiceScheduleMap,
} from "../lib/types";

export type EditSaveStatus = "idle" | "saving" | "saved";

interface ClientGroupCardProps {
  g: ClientGroup;
  isMobile: boolean;

  // edit state
  editingId: number | null;
  editName: string;
  editLabel: string;
  editBlocked: boolean;
  editRules: string;
  scopeType?: "global" | "group" | "client";
  scopeKey?: string;
  editTab: "rules" | "services";
  editSaveStatus: EditSaveStatus;
  setEditName: (v: string) => void;
  setEditLabel: (v: string) => void;
  setEditTab: (v: "rules" | "services") => void;
  setEditRules: (v: string) => void;
  setEditingId: (id: number | null) => void;
  onStartEdit: (g: ClientGroup) => void;
  onSaveEdit: (id: number, patch?: Record<string, unknown>) => void;
  onEditBlockedChange: (val: boolean, id: number) => void;
  onDelete: (id: number, name: string) => void;

  // member state
  addMemberGroupId: number | null;
  memberInput: string;
  memberType: "ip" | "mac";
  setAddMemberGroupId: (id: number | null) => void;
  setMemberInput: (v: string) => void;
  setMemberType: (v: "ip" | "mac") => void;
  onAddMember: (groupId: number) => void;
  onRemoveMember: (groupId: number, identifier: string) => void;

  // service schedule state
  svcDefs: ServiceDef[];
  groupSvcSchedules: Record<string, ServiceScheduleMap>;
  svcSaving: string | null;
  svcLoading: boolean;
  onFetchGroupSvcSchedules: (groupId: number) => void;
  onSaveGroupSvcSchedule: (
    groupId: number,
    svcId: string,
    patch: Partial<{
      enabled: boolean;
      days_of_week: string;
      time_start: string;
      time_end: string;
    }>,
  ) => void;
  onResetGroupSvcSchedule: (groupId: number, svcId: string) => void;
}

const ClientGroupCard: React.FC<ClientGroupCardProps> = ({
  g,
  isMobile,
  editingId,
  editName,
  editLabel,
  editBlocked,
  editRules,
  scopeType,
  scopeKey,
  editTab,
  editSaveStatus,
  setEditName,
  setEditLabel,
  setEditTab,
  setEditRules,
  setEditingId,
  onStartEdit,
  onSaveEdit,
  onEditBlockedChange,
  onDelete,
  addMemberGroupId,
  memberInput,
  memberType,
  setAddMemberGroupId,
  setMemberInput,
  setMemberType,
  onAddMember,
  onRemoveMember,
  svcDefs,
  groupSvcSchedules,
  svcSaving,
  svcLoading,
  onFetchGroupSvcSchedules,
  onSaveGroupSvcSchedule,
  onResetGroupSvcSchedule,
}) => {
  const isEditing = editingId === g.id;
  const isAddingMember = addMemberGroupId === g.id;
  const svcKey = `group:${g.id}` as const;
  const clientSched = groupSvcSchedules[svcKey] ?? {};

  const blockedSvcCount = svcDefs.filter((def) => {
    const s = clientSched[def.id];
    if (s?.source === "client") return s.enabled;
    if (s?.source === "global") return s.enabled;
    return false;
  }).length;

  const chipStatus = (allowed: boolean) => (
    <span
      className="text-[10px] font-bold px-1.75 py-px rounded-[10px] border"
      style={{
        background: allowed ? "#1a211a" : "#2a1414",
        color: allowed ? ACCENT : "#c0392b",
        borderColor: allowed ? "#3a5a3a" : "#7a3333",
      }}
    >
      {allowed ? "Allowed" : "Blocked"}
    </span>
  );

  const inputCls =
    "w-full px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border";
  const textareaCls = `${inputCls} font-mono resize-y`;
  const fieldLabelCls =
    "block text-[11px] font-semibold text-text-faint mb-1 uppercase tracking-[0.04em]";

  return (
    <div
      className="border rounded-lg mb-2.5 overflow-hidden"
      style={{
        borderColor: "#222",
        background: isEditing ? "#141414" : "#0f0f0f",
      }}
    >
      {/* ── row header ── */}
      <div
        className="flex items-center gap-2.5 px-3.5 py-2.5"
        style={{ borderBottom: isEditing ? "1px solid #1e1e1e" : "none" }}
      >
        <FaNetworkWired className="text-text-ghost shrink-0" />
        <div className="flex-1 min-w-0">
          {/* name + status chips */}
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-[13px] font-bold text-[#c8d4c6]">
              {g.name}
            </span>
            {g.label && (
              <span className="text-[12px] text-text-faint">{g.label}</span>
            )}
            <span className="text-[10px] px-1.75 py-px rounded-[10px] bg-[#1a1a2a] text-[#7a8fa0] border border-[#2a2a3a]">
              {scopeType ?? "group"}:{scopeKey ?? g.id}
            </span>
            {chipStatus(!g.blocked)}
            <span className="text-[10px] px-1.75 py-px rounded-[10px] bg-[#1a1a2a] text-[#7a8fa0] border border-[#2a2a3a]">
              {g.members.length} {g.members.length === 1 ? "device" : "devices"}
            </span>
            {blockedSvcCount > 0 && (
              <span
                className="text-[10px] px-1.75 py-px rounded-[10px] bg-danger-dim text-danger border border-[#7a333344]"
                title={`${blockedSvcCount} service${blockedSvcCount !== 1 ? "s" : ""} blocked`}
              >
                🔒 {blockedSvcCount} blocked
              </span>
            )}
          </div>

          {/* member chips */}
          <div className="flex flex-wrap gap-1 mt-1.25">
            {g.members.map((m: GroupMember) => (
              <span
                key={m.id}
                className="inline-flex items-center gap-1 text-[11px] font-mono px-2 py-0.5 rounded bg-surface-1 border border-border-mid"
                style={{ color: m.type === "mac" ? "#7a8fa0" : "#c8d4c6" }}
              >
                {m.type === "mac" ? "⬡" : "●"} {m.identifier}
                <button
                  onClick={() => onRemoveMember(g.id, m.identifier)}
                  className="bg-transparent border-none text-text-ghost cursor-pointer p-0 text-[10px] leading-none ml-0.5"
                  title="Remove"
                >
                  ×
                </button>
              </span>
            ))}
            <button
              onClick={() => setAddMemberGroupId(isAddingMember ? null : g.id)}
              className="inline-flex items-center gap-0.75 text-[11px] px-2 py-0.5 rounded-sm bg-transparent cursor-pointer"
              style={{
                border: `1px dashed ${isAddingMember ? ACCENT : "#333"}`,
                color: isAddingMember ? ACCENT : "#555",
              }}
            >
              <FaPlus className="text-[8px]" /> Add
            </button>
          </div>

          {/* inline add-member row */}
          {isAddingMember && (
            <div className="flex items-center gap-1.5 mt-2 flex-wrap">
              <select
                value={memberType}
                onChange={(e) => setMemberType(e.target.value as "ip" | "mac")}
                className="px-2 py-1.25 bg-surface-2 border border-border-mid rounded-md text-text text-[12px] outline-none"
              >
                <option value="ip">IP</option>
                <option value="mac">MAC</option>
              </select>
              <input
                value={memberInput}
                onChange={(e) => setMemberInput(e.target.value)}
                placeholder={
                  memberType === "ip" ? "192.168.1.x" : "aa:bb:cc:dd:ee:ff"
                }
                className="flex-1 min-w-35 px-2.5 py-1.25 border border-[#333] rounded-md bg-surface-2 text-text text-[12px] outline-none box-border"
                onKeyDown={(e) => {
                  if (e.key === "Enter") onAddMember(g.id);
                }}
              />
              <button
                onClick={() => onAddMember(g.id)}
                disabled={!memberInput.trim()}
                className="px-3 py-1.25 text-white border-none rounded-md cursor-pointer text-[12px] font-semibold disabled:opacity-50"
                style={{ background: ACCENT }}
              >
                Add
              </button>
              <button
                onClick={() => {
                  setAddMemberGroupId(null);
                  setMemberInput("");
                }}
                className="px-2.5 py-1.25 bg-transparent text-text-muted border border-border-mid rounded-md cursor-pointer text-[12px]"
              >
                Cancel
              </button>
            </div>
          )}
        </div>

        {/* action buttons */}
        <div className="flex gap-1.5 shrink-0">
          <button
            onClick={() => (isEditing ? setEditingId(null) : onStartEdit(g))}
            className="flex items-center justify-center w-7 h-7 p-0 border border-border-mid rounded text-text-muted cursor-pointer text-[12px] shrink-0"
            style={{ background: isEditing ? "#222" : "transparent" }}
          >
            <FaEdit />
          </button>
          <button
            onClick={() => onDelete(g.id, g.name)}
            className="flex items-center justify-center w-7 h-7 p-0 bg-transparent border border-border-mid rounded text-danger-border cursor-pointer text-[12px] shrink-0"
          >
            <FaTrash />
          </button>
        </div>
      </div>

      {/* ── edit panel ── */}
      {isEditing && (
        <div className="p-3.5">
          <div
            className={`grid gap-3 mb-3 ${isMobile ? "grid-cols-1" : "grid-cols-2"}`}
          >
            <div>
              <label className={fieldLabelCls}>Name</label>
              <input
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                onBlur={() => onSaveEdit(g.id)}
                className={inputCls}
              />
            </div>
            <div>
              <label className={fieldLabelCls}>Description</label>
              <input
                value={editLabel}
                onChange={(e) => setEditLabel(e.target.value)}
                onBlur={() => onSaveEdit(g.id)}
                className={inputCls}
              />
            </div>
          </div>

          <div className="mb-3.5">
            <ToggleSwitch
              checked={editBlocked}
              onChange={(val) => onEditBlockedChange(val, g.id)}
              label="Block all DNS for this client"
            />
          </div>

          {/* tabs */}
          <div className="flex gap-0 mb-3.5 border-b border-border-dim">
            <button
              onClick={() => setEditTab("rules")}
              className="px-3.5 py-1.5 bg-transparent border-none cursor-pointer text-[12px] font-semibold -mb-px"
              style={{
                borderBottom: `2px solid ${editTab === "rules" ? ACCENT : "transparent"}`,
                color: editTab === "rules" ? ACCENT : "#555",
              }}
            >
              Custom Rules
            </button>
            <button
              onClick={() => {
                setEditTab("services");
                onFetchGroupSvcSchedules(g.id);
              }}
              className="px-3.5 py-1.5 bg-transparent border-none cursor-pointer text-[12px] font-semibold -mb-px flex items-center gap-1.25"
              style={{
                borderBottom: `2px solid ${editTab === "services" ? ACCENT : "transparent"}`,
                color: editTab === "services" ? ACCENT : "#555",
              }}
            >
              Service Blocks
              {blockedSvcCount > 0 && (
                <span
                  className="text-[9px] font-bold px-1.25 py-px rounded-lg text-danger border border-[#7a333333]"
                  style={{
                    background: editTab === "services" ? "#2a1414" : "#1e1414",
                  }}
                >
                  {blockedSvcCount}
                </span>
              )}
            </button>
          </div>

          {/* rules tab */}
          {editTab === "rules" && (
            <div className="mb-3">
              <textarea
                value={editRules}
                onChange={(e) => setEditRules(e.target.value)}
                onBlur={() => onSaveEdit(g.id)}
                placeholder={
                  "! Block a domain\n||ads.example.com^\n\n! Allow a domain\n@@||safe.example.com^"
                }
                className={`${textareaCls} min-h-35`}
              />
              <RulesErrors rules={editRules} />
              <span className="text-[11px] text-text-ghost">
                <code className="text-danger">||domain^</code> block{"  ·  "}
                <code className="text-accent">@@||domain^</code> allow
                {"  ·  "}
                <code className="text-text-ghost"># comment</code>
              </span>
            </div>
          )}

          {/* services tab */}
          {editTab === "services" && (
            <div className="mb-3">
              <p className="text-[11px] text-text-ghost mt-0 mb-2.5">
                Services showing{" "}
                <strong className="text-text-ghost">Inherit</strong> follow the
                global schedule. Click any service to create a client-specific
                override — set it{" "}
                <strong className="text-danger">Blocked</strong> to always block
                for this client, or{" "}
                <strong style={{ color: ACCENT }}>Blocked off</strong> to
                explicitly allow it even when blocked globally. Use{" "}
                <strong className="text-text-ghost">↩ Inherit</strong> to remove
                the override and go back to global.
              </p>
              {svcLoading ? (
                <div className="flex items-center gap-2 py-5 text-text-ghost text-[12px]">
                  <span
                    className="inline-block w-3.5 h-3.5 rounded-full border-2 border-border-mid spin-slow"
                    style={{ borderTopColor: ACCENT }}
                  />
                  Loading service schedules…
                </div>
              ) : svcDefs.length === 0 ? (
                <div className="text-text-dead text-[12px] py-3">
                  No service definitions found. Check server connection.
                </div>
              ) : (
                <ServiceBlockList
                  scope="group"
                  defs={svcDefs}
                  schedules={groupSvcSchedules[`group:${g.id}`] ?? {}}
                  saving={svcSaving}
                  onSave={(svcId, patch) =>
                    onSaveGroupSvcSchedule(g.id, svcId, patch)
                  }
                  onReset={(svcId) => onResetGroupSvcSchedule(g.id, svcId)}
                />
              )}
            </div>
          )}

          {/* save status + close */}
          <div className="flex items-center gap-2.5 mt-2">
            <span
              className="text-[11px] flex items-center gap-1 select-none transition-[color] duration-[0.4s]"
              style={{
                color:
                  editSaveStatus === "saved"
                    ? ACCENT
                    : editSaveStatus === "saving"
                      ? "#555"
                      : "transparent",
              }}
            >
              {editSaveStatus === "saving" ? (
                <>
                  <span
                    className="inline-block w-2.5 h-2.5 rounded-full border-2 border-[#333] spin-slow"
                    style={{ borderTopColor: "#555" }}
                  />
                  Saving…
                </>
              ) : (
                <>
                  <FaCheck className="text-[9px]" /> Saved
                </>
              )}
            </span>
            <button
              onClick={() => setEditingId(null)}
              className="ml-auto px-3 py-1.5 bg-transparent text-text-ghost border border-border-mid rounded-md cursor-pointer text-[12px]"
            >
              Close
            </button>
          </div>
        </div>
      )}
    </div>
  );
};

export default ClientGroupCard;
