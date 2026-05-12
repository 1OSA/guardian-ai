import React, { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import axios from "axios";
import { FaGamepad, FaGlobe } from "react-icons/fa";
import { useMediaQuery } from "../lib/useMediaQuery";
import { ACCENT } from "../lib/constants";
import ServiceBlockList from "../components/ServiceBlockList";
import type {
  ServiceDef,
  ServiceSchedule,
  ServiceScheduleMap,
} from "../lib/types";

const ServicesPage: React.FC = () => {
  const isMobile = useMediaQuery("(max-width: 768px)");

  const [defs, setDefs] = useState<ServiceDef[]>([]);
  const [globalSchedules, setGlobalSchedules] = useState<ServiceScheduleMap>(
    {},
  );
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([
      axios.get("/api/services/definitions").then((r) => setDefs(r.data)),
      axios
        .get("/api/services?scope=global")
        .then((r) => setGlobalSchedules(r.data)),
    ]).finally(() => setLoading(false));
  }, []);

  const saveSchedule = async (
    svcId: string,
    patch: Partial<ServiceSchedule>,
  ) => {
    const current: ServiceSchedule = globalSchedules[svcId] ?? {
      enabled: false,
      days_of_week: "",
      time_start: "",
      time_end: "",
    };

    // Explicit boolean casting to prevent API misinterpretation
    const next: ServiceSchedule = {
      ...current,
      ...patch,
      enabled:
        patch.enabled !== undefined ? Boolean(patch.enabled) : current.enabled,
    };

    setSaving(svcId);
    try {
      await axios.post("/api/services", {
        scope: "global",
        scope_key: "",
        service_id: svcId,
        enabled: next.enabled,
        days_of_week: next.days_of_week,
        time_start: next.time_start,
        time_end: next.time_end,
      });
      setGlobalSchedules((prev) => ({ ...prev, [svcId]: next }));
    } catch {
      /* ignore */
    } finally {
      setSaving(null);
    }
  };

  if (loading) {
    return <div className="p-6 text-text-ghost">Loading…</div>;
  }

  return (
    <div className={isMobile ? "p-3" : "p-6"}>
      {/* header */}
      <div className="flex items-center gap-2 mb-1">
        <FaGamepad style={{ color: ACCENT, fontSize: 16 }} />
        <span className="text-xl font-bold text-text">Services</span>
      </div>
      <p className="text-[12px] text-text-ghost mt-1 mb-5">
        Global blocks apply to all clients. Per-client overrides can be set
        inside{" "}
        <Link to="/clients" className="no-underline" style={{ color: ACCENT }}>
          Clients
        </Link>{" "}
        → edit a client → <strong className="text-text">Service Blocks</strong>{" "}
        tab.
      </p>

      <div className="bg-surface-1 text-text rounded-[10px] border border-border shadow-card p-5 mb-4">
        <div className="flex items-center gap-2 mb-3.5">
          <FaGlobe style={{ color: ACCENT, fontSize: 13 }} />
          <span className="text-[14px] font-bold text-text">
            Global Service Blocks
          </span>
        </div>
        <ServiceBlockList
          scope="global"
          defs={defs}
          schedules={globalSchedules}
          saving={saving}
          onSave={saveSchedule}
        />
      </div>
    </div>
  );
};

export default ServicesPage;
