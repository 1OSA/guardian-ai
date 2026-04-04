import React, { useCallback, useEffect, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import axios from "axios";
import { FaUsers, FaNetworkWired, FaPlus } from "react-icons/fa";
import { useMediaQuery } from "../lib/useMediaQuery";
import { ACCENT } from "../lib/constants";
import ClientAddForm from "../components/ClientAddForm";
import ClientGroupCard from "../components/ClientGroupCard";
import type {
  ClientGroup,
  ServiceDef,
  ServiceSchedule,
  ServiceScheduleMap,
} from "../lib/types";

type UnifiedRuleScope = "global" | "group" | "client";

const ClientsPage: React.FC = () => {
  const isMobile = useMediaQuery("(max-width: 768px)");
  const location = useLocation();
  const navigate = useNavigate();

  const [groups, setGroups] = useState<ClientGroup[]>([]);
  const [loading, setLoading] = useState(false);
  const [clientSearch, setClientSearch] = useState("");

  // add form
  const [showAdd, setShowAdd] = useState(false);
  const [formName, setFormName] = useState("");
  const [formLabel, setFormLabel] = useState("");
  const [formBlocked, setFormBlocked] = useState(false);
  const [formRules, setFormRules] = useState("");
  const [formMembers, setFormMembers] = useState("");
  const [saving, setSaving] = useState(false);
  const [editSaveStatus, setEditSaveStatus] = useState<
    "idle" | "saving" | "saved"
  >("idle");
  const editSaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // edit state
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editName, setEditName] = useState("");
  const [editLabel, setEditLabel] = useState("");
  const [editBlocked, setEditBlocked] = useState(false);
  const [editRules, setEditRules] = useState("");
  const [editTab, setEditTab] = useState<"rules" | "services">("rules");

  // member add
  const [addMemberGroupId, setAddMemberGroupId] = useState<number | null>(null);
  const [memberInput, setMemberInput] = useState("");
  const [memberType, setMemberType] = useState<"ip" | "mac">("ip");

  // service schedules
  const [svcDefs, setSvcDefs] = useState<ServiceDef[]>([]);
  const [groupSvcSchedules, setGroupSvcSchedules] = useState<
    Record<string, ServiceScheduleMap>
  >({});
  const [svcSaving, setSvcSaving] = useState<string | null>(null);
  const [svcLoading, setSvcLoading] = useState(false);

  const fetchGroupSvcSchedules = useCallback(async (groupId: number) => {
    const key = `group:${groupId}`;
    setSvcLoading(true);
    try {
      const res = await axios.get(
        `/api/services?scope=group&key=${encodeURIComponent(key)}&merged=1`,
      );
      setGroupSvcSchedules((prev) => ({
        ...prev,
        [key]: res.data as ServiceScheduleMap,
      }));
    } catch {
      /* ignore */
    } finally {
      setSvcLoading(false);
    }
  }, []);

  const upsertRules = useCallback(
    async (
      scopeType: UnifiedRuleScope,
      scopeKey: string,
      label: string,
      blocked: boolean,
      rules: string,
    ) => {
      await axios.post("/api/rules", {
        scope_type: scopeType,
        scope_key: scopeKey,
        label,
        blocked,
        rules,
      });
    },
    [],
  );

  const loadRules = useCallback(
    async (scopeType: UnifiedRuleScope, scopeKey: string) => {
      try {
        const res = await axios.get(
          `/api/rules?scope=${scopeType}&key=${encodeURIComponent(scopeKey)}`,
        );
        return res.data.rules || "";
      } catch {
        return "";
      }
    },
    [],
  );

  const startEditGroup = useCallback(
    async (g: ClientGroup, initialTab: "rules" | "services" = "rules") => {
      setEditingId(g.id);
      setEditName(g.name);
      setEditLabel(g.label);
      setEditBlocked(g.blocked);
      const rules = await loadRules("group", g.id.toString());
      setEditRules(rules);
      setEditTab(initialTab);
      setEditSaveStatus("idle");
      fetchGroupSvcSchedules(g.id);
    },
    [loadRules, fetchGroupSvcSchedules],
  );

  const fetchGroups = useCallback(async (): Promise<ClientGroup[]> => {
    setLoading(true);
    try {
      const res = await axios.get("/api/groups");
      const data = res.data as ClientGroup[];
      setGroups(data);
      return data;
    } catch {
      return [];
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchSvcDefs = useCallback(async () => {
    try {
      const res = await axios.get("/api/services/definitions");
      setSvcDefs(res.data as ServiceDef[]);
    } catch {
      /* ignore */
    }
  }, []);

  // ── deep-link: ?edit=<ip>[&tab=services] ─────────────────────────────────
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const editIp = params.get("edit");
    if (!editIp) return;
    const tab = params.get("tab") === "services" ? "services" : "rules";
    navigate("/clients", { replace: true });
    fetchGroups().then(async (loaded) => {
      const g = loaded.find((gr) =>
        gr.members.some((m) => m.identifier === editIp),
      );
      if (g) await startEditGroup(g, tab);
      else {
        try {
          const res = await axios.post("/api/groups", {
            name: editIp,
            label: "",
            blocked: false,
          });
          const newId = (res.data as { id: number }).id;
          const mType =
            editIp.includes(":") && editIp.length === 17 ? "mac" : "ip";
          await axios.post("/api/groups/members", {
            group_id: newId,
            identifier: editIp,
            type: mType,
          });
          const refreshed = await fetchGroups();
          const created = refreshed.find((gr) => gr.id === newId);
          if (created) await startEditGroup(created, tab);
        } catch {
          setFormName(editIp);
          setFormMembers(editIp);
          setFormLabel("");
          setFormRules("");
          setFormBlocked(false);
          setShowAdd(true);
        }
      }
    });
  }, [location.search, navigate, fetchGroups, startEditGroup]);

  useEffect(() => {
    fetchGroups();
    fetchSvcDefs();
  }, [fetchGroups, fetchSvcDefs]);

  useEffect(
    () => () => {
      if (editSaveTimerRef.current) clearTimeout(editSaveTimerRef.current);
    },
    [],
  );

  // ── helpers ───────────────────────────────────────────────────────────────

  const parseMembers = (raw: string) =>
    raw
      .split(/[\n,]+/)
      .map((s) => s.trim().toLowerCase())
      .filter(Boolean)
      .map((s) => ({
        identifier: s,
        type: (s.includes(":") && s.length === 17 ? "mac" : "ip") as
          | "ip"
          | "mac",
      }));

  // ── actions ───────────────────────────────────────────────────────────────

  const saveNew = async () => {
    if (!formName.trim()) return;
    setSaving(true);
    try {
      const res = await axios.post("/api/groups", {
        name: formName.trim(),
        label: formLabel.trim(),
        blocked: formBlocked,
      });
      const newId = (res.data as { id: number }).id;
      for (const m of parseMembers(formMembers)) {
        await axios.post("/api/groups/members", {
          group_id: newId,
          identifier: m.identifier,
          type: m.type,
        });
      }
      if (formRules.trim()) {
        await upsertRules(
          "group",
          newId.toString(),
          formLabel.trim(),
          formBlocked,
          formRules,
        );
      }
      setFormName("");
      setFormLabel("");
      setFormBlocked(false);
      setFormRules("");
      setFormMembers("");
      setShowAdd(false);
      const loaded = await fetchGroups();
      const created = loaded.find((gr) => gr.id === newId);
      if (created) startEditGroup(created);
    } catch {
      /* ignore */
    } finally {
      setSaving(false);
    }
  };

  const saveEdit = async (
    id: number,
    fields?: {
      name?: string;
      label?: string;
      blocked?: boolean;
    },
  ) => {
    setEditSaveStatus("saving");
    try {
      const nextName = fields?.name ?? editName;
      const nextLabel = fields?.label ?? editLabel;
      const nextBlocked = fields?.blocked ?? editBlocked;
      await axios.post("/api/groups", {
        id,
        name: nextName,
        label: nextLabel,
        blocked: nextBlocked,
      });
      if (editRules.trim()) {
        await upsertRules(
          "group",
          id.toString(),
          nextLabel,
          nextBlocked,
          editRules,
        );
      }
      await fetchGroups();
      setEditSaveStatus("saved");
      if (editSaveTimerRef.current) clearTimeout(editSaveTimerRef.current);
      editSaveTimerRef.current = setTimeout(
        () => setEditSaveStatus("idle"),
        2000,
      );
    } catch {
      setEditSaveStatus("idle");
    }
  };

  const deleteGroup = async (id: number, name: string) => {
    if (!confirm(`Delete "${name}"? All members will also be removed.`)) return;
    try {
      await axios.delete("/api/groups", { data: { id } });
      await fetchGroups();
    } catch {
      /* ignore */
    }
  };

  const handleEditBlockedChange = (val: boolean, id: number) => {
    setEditBlocked(val);
    saveEdit(id, { blocked: val });
  };

  const addMember = async (groupId: number) => {
    const ident = memberInput.trim();
    if (!ident) return;
    try {
      await axios.post("/api/groups/members", {
        group_id: groupId,
        identifier: ident,
        type: memberType,
      });
      setMemberInput("");
      setAddMemberGroupId(null);
      await fetchGroups();
    } catch {
      /* ignore */
    }
  };

  const removeMember = async (groupId: number, identifier: string) => {
    try {
      await axios.delete("/api/groups/members", {
        data: { group_id: groupId, identifier },
      });
      await fetchGroups();
    } catch {
      /* ignore */
    }
  };

  const resetGroupSvcSchedule = async (groupId: number, svcId: string) => {
    const key = `group:${groupId}`;
    setSvcSaving(svcId);
    try {
      await axios.delete("/api/services", {
        data: { scope: "group", scope_key: key, service_id: svcId },
      });
      await fetchGroupSvcSchedules(groupId);
    } catch {
      /* ignore */
    } finally {
      setSvcSaving(null);
    }
  };

  const saveGroupSvcSchedule = async (
    groupId: number,
    svcId: string,
    patch: Partial<ServiceSchedule>,
  ) => {
    const key = `group:${groupId}`;
    setSvcSaving(svcId);
    try {
      // Build the next row locally to post to the server.
      const current: ServiceSchedule = groupSvcSchedules[key]?.[svcId] ?? {
        enabled: false,
        days_of_week: "",
        time_start: "",
        time_end: "",
      };
      const next: ServiceSchedule = { ...current, ...patch };

      // Persist to the server.
      await axios.post("/api/services", {
        scope: "group",
        scope_key: key,
        service_id: svcId,
        enabled: next.enabled,
        days_of_week: next.days_of_week,
        time_start: next.time_start,
        time_end: next.time_end,
      });

      // After saving, refresh the schedules from the server (no optimistic update).
      await fetchGroupSvcSchedules(groupId);
    } catch {
      /* ignore */
    } finally {
      setSvcSaving(null);
    }
  };

  // ── derived ───────────────────────────────────────────────────────────────

  const visibleGroups = groups.filter((g) => {
    if (!clientSearch.trim()) return true;
    const s = clientSearch.toLowerCase();
    return (
      g.name.toLowerCase().includes(s) ||
      g.label.toLowerCase().includes(s) ||
      g.members.some((m) => m.identifier.toLowerCase().includes(s))
    );
  });

  // ── render ────────────────────────────────────────────────────────────────

  return (
    <div className={isMobile ? "p-3" : "p-6"}>
      {/* page header */}
      <div className="flex items-center gap-2 mb-5">
        <FaUsers style={{ color: ACCENT, fontSize: 16 }} />
        <span className="text-xl font-bold text-text">Clients</span>
      </div>

      <div className="bg-surface-1 text-text rounded-[10px] border border-border shadow-[0_2px_8px_rgba(0,0,0,0.3)] p-5 mb-4">
        {/* card header */}
        <div className="flex items-center justify-between mb-3.5">
          <div className="flex items-center gap-2">
            <FaNetworkWired style={{ color: ACCENT }} />
            <span className="text-[15px] font-bold text-text">
              Client Rules
            </span>
          </div>
          <button
            onClick={() => setShowAdd((v) => !v)}
            className="flex items-center gap-1.5 px-3 py-1.5 border rounded-md text-[12px] font-semibold cursor-pointer"
            style={{
              background: showAdd ? "#111" : "#1a211a",
              border: `1px solid ${ACCENT}`,
              color: ACCENT,
            }}
          >
            <FaPlus className="text-[10px]" /> Add Client
          </button>
        </div>

        {/* add form */}
        {showAdd && (
          <ClientAddForm
            isMobile={isMobile}
            formName={formName}
            setFormName={setFormName}
            formLabel={formLabel}
            setFormLabel={setFormLabel}
            formMembers={formMembers}
            setFormMembers={setFormMembers}
            formRules={formRules}
            setFormRules={setFormRules}
            formBlocked={formBlocked}
            setFormBlocked={setFormBlocked}
            saving={saving}
            onSave={saveNew}
            onCancel={() => setShowAdd(false)}
          />
        )}

        {/* search */}
        {groups.length > 0 && (
          <div className="mb-3">
            <input
              value={clientSearch}
              onChange={(e) => setClientSearch(e.target.value)}
              placeholder="Search by name, description, or IP / MAC…"
              className="w-full px-2.5 py-2 border border-[#333] rounded-md bg-surface-2 text-text text-[13px] outline-none box-border"
            />
          </div>
        )}

        {/* client list */}
        {loading ? (
          <div className="text-text-ghost text-[13px] p-3">Loading…</div>
        ) : groups.length === 0 && !showAdd ? (
          <div className="text-text-dead text-[13px] py-5 text-center">
            No clients yet. Add one above to apply rules per device or group of
            devices.
          </div>
        ) : visibleGroups.length === 0 ? (
          <div className="text-text-dead text-[13px] py-5 text-center">
            No clients match "
            <strong className="text-text-faint">{clientSearch}</strong>".
          </div>
        ) : (
          visibleGroups.map((g) => (
            <ClientGroupCard
              key={g.id}
              g={g}
              isMobile={isMobile}
              editingId={editingId}
              editName={editName}
              editLabel={editLabel}
              editBlocked={editBlocked}
              editRules={editRules}
              editTab={editTab}
              editSaveStatus={editSaveStatus}
              setEditName={setEditName}
              setEditLabel={setEditLabel}
              setEditTab={setEditTab}
              setEditRules={setEditRules}
              setEditingId={setEditingId}
              onStartEdit={startEditGroup}
              onSaveEdit={saveEdit}
              onEditBlockedChange={handleEditBlockedChange}
              onDelete={deleteGroup}
              addMemberGroupId={addMemberGroupId}
              memberInput={memberInput}
              memberType={memberType}
              setAddMemberGroupId={setAddMemberGroupId}
              setMemberInput={setMemberInput}
              setMemberType={setMemberType}
              onAddMember={addMember}
              onRemoveMember={removeMember}
              svcDefs={svcDefs}
              groupSvcSchedules={groupSvcSchedules}
              svcSaving={svcSaving}
              svcLoading={svcLoading}
              onFetchGroupSvcSchedules={fetchGroupSvcSchedules}
              onSaveGroupSvcSchedule={saveGroupSvcSchedule}
              onResetGroupSvcSchedule={resetGroupSvcSchedule}
            />
          ))
        )}
      </div>
    </div>
  );
};

export default ClientsPage;
