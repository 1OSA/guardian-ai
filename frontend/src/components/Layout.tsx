import React, { useEffect, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import axios from "axios";
import {
  FaHome,
  FaList,
  FaCog,
  FaSignOutAlt,
  FaUser,
  FaBars,
  FaTimes,
  FaUsers,
  FaBrain,
  FaGamepad,
} from "react-icons/fa";
import ThemeToggle from "./ThemeToggle";
import { useAuth } from "../lib/AuthContext";
import { useMediaQuery } from "../lib/useMediaQuery";
import { ACCENT, SIDEBAR_W } from "../lib/constants";

declare const __APP_VERSION__: string;

const Layout: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { logout, user } = useAuth();
  const location = useLocation();
  const isMobile = useMediaQuery("(max-width: 768px)");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [appVersion, setAppVersion] = useState<string>(
    typeof __APP_VERSION__ !== "undefined" ? __APP_VERSION__ : "dev",
  );

  useEffect(() => {
    axios
      .get<{ version: string }>("/api/version")
      .then((r) => setAppVersion(r.data.version))
      .catch(() => {
        /* keep build-time version */
      });
  }, []);

  const navItems = [
    { path: "/dashboard", label: "Dashboard", icon: <FaHome /> },
    { path: "/queries", label: "Query Log", icon: <FaList /> },
    { path: "/clients", label: "Clients", icon: <FaUsers /> },
    { path: "/services", label: "Services", icon: <FaGamepad /> },
    { path: "/ml", label: "Threat Detection", icon: <FaBrain /> },
    { path: "/settings", label: "Settings", icon: <FaCog /> },
  ];

  const closeSidebar = () => setSidebarOpen(false);

  const sidebarContent = (
    <div
      className="flex flex-col h-full bg-surface-2 border-r border-border-dim box-border"
      style={{ width: SIDEBAR_W, minWidth: SIDEBAR_W }}
    >
      {/* logo */}
      <div className="px-5 pt-5.5 pb-4.5 border-b border-border-dim flex items-center justify-between">
        <div className="flex items-center gap-2.25">
          <img
            src="/favicon.svg"
            alt="Guardian AI"
            style={{ width: 20, height: 20, flexShrink: 0 }}
          />
          <div className="text-[15px] font-bold text-text">Guardian AI</div>
        </div>
        {isMobile && (
          <button
            onClick={closeSidebar}
            className="bg-transparent border-none text-text-ghost text-lg cursor-pointer p-1"
          >
            <FaTimes />
          </button>
        )}
      </div>

      {/* nav */}
      <nav className="flex-1 px-2.5 py-3">
        {navItems.map((item) => {
          const active = location.pathname === item.path;
          return (
            <Link
              key={item.path}
              to={item.path}
              onClick={() => isMobile && closeSidebar()}
              className="flex items-center gap-2.75 px-3 py-2.5 mb-0.5 rounded-lg no-underline text-[13px] transition-[background,color] duration-150 box-border"
              style={{
                color: active ? "var(--color-text)" : "var(--color-text-faint)",
                background: active ? "var(--color-surface-1)" : "transparent",
                borderLeft: `3px solid ${active ? ACCENT : "transparent"}`,
                fontWeight: active ? 600 : 400,
              }}
            >
              <span
                className="text-[15px] shrink-0 transition-colors duration-150"
                style={{ color: active ? ACCENT : "var(--color-text-ghost)" }}
              >
                {item.icon}
              </span>
              {item.label}
            </Link>
          );
        })}
      </nav>

      {/* bottom: theme + version + user + logout */}
      <div className="border-t border-border-dim px-2.5 py-3">
        {/* theme toggle + version */}
        <div className="flex items-center justify-between gap-2 px-1 mb-2">
          <ThemeToggle />
          <span className="text-[10px] font-semibold px-2 py-0.5 rounded-full bg-surface-1 text-text-dead border border-border tracking-[0.05em]">
            v{appVersion}
          </span>
        </div>

        {/* user chip */}
        <div className="flex items-center gap-2.25 px-3 py-2 rounded-lg bg-surface-1 border border-border mb-1.5">
          <div
            className="w-7 h-7 rounded-full bg-accent-dim flex items-center justify-center shrink-0"
            style={{ border: `1px solid ${ACCENT}44` }}
          >
            <FaUser style={{ fontSize: 12, color: ACCENT }} />
          </div>
          <div className="min-w-0">
            <div className="text-[12px] font-semibold text-text-dim overflow-hidden text-ellipsis whitespace-nowrap">
              {user ?? "—"}
            </div>
            <div className="text-[10px] text-text-dead">Administrator</div>
          </div>
        </div>

        {/* logout */}
        <button
          onClick={logout}
          className="flex items-center gap-2.5 w-full px-3 py-2.25 bg-transparent border border-border-dim rounded-lg text-text-ghost cursor-pointer text-[13px] font-medium transition-[background,color,border-color] duration-150 box-border hover:bg-danger-dim hover:text-danger hover:border-danger-border"
        >
          <FaSignOutAlt className="text-sm shrink-0" />
          Sign Out
        </button>
      </div>
    </div>
  );

  return (
    <div className="flex h-screen bg-surface text-text overflow-hidden">
      {/* mobile overlay */}
      {isMobile && sidebarOpen && (
        <div
          onClick={closeSidebar}
          className="fixed inset-0 bg-black/60 z-999"
        />
      )}

      {/* sidebar — fixed on mobile, static on desktop */}
      {isMobile ? (
        <div
          className="fixed top-0 h-full z-1000 transition-[left] duration-250 ease-in-out"
          style={{ left: sidebarOpen ? 0 : -SIDEBAR_W }}
        >
          {sidebarContent}
        </div>
      ) : (
        sidebarContent
      )}

      {/* main content */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* mobile-only topbar (hamburger to open sidebar) */}
        {isMobile && (
          <div className="h-13 border-b border-border-dim bg-surface-2 flex items-center px-5 gap-3 shrink-0">
            <button
              onClick={() => setSidebarOpen(true)}
              className="bg-transparent border-none text-text-muted text-lg cursor-pointer p-1 flex items-center"
            >
              <FaBars />
            </button>
          </div>
        )}

        {/* page content */}
        <div className="flex-1 overflow-y-auto">{children}</div>
      </div>
    </div>
  );
};

export default Layout;
