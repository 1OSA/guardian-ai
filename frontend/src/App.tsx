import React, { useEffect, useState } from "react";
import { Routes, Route, Navigate, useNavigate } from "react-router-dom";
import axios from "axios";

import { ProtectedRoute, useAuth } from "./lib/AuthContext";
import Layout from "./components/Layout";

import LoginPage from "./pages/LoginPage";
import DashboardPage from "./pages/DashboardPage";
import QueriesPage from "./pages/QueriesPage";
import ClientsPage from "./pages/ClientsPage";
import ServicesPage from "./pages/ServicesPage";
import MLPage from "./pages/MLPage";
import SettingsPage from "./pages/SettingsPage";
import SetupWizard from "./pages/SetupWizard";

/* ---------- Setup Wizard Wrapper (auto-login after setup) ---------- */

const SetupWizardWrapper: React.FC<{ onComplete: () => void }> = ({
  onComplete,
}) => {
  const { refreshAuth } = useAuth();
  const navigate = useNavigate();

  const handleComplete = async (_username: string, _password: string) => {
    // Setup endpoint creates a session and sets the cookie automatically
    // Just refresh the auth context to pick up the new session
    await refreshAuth();
    // Wait a bit for React to process the state updates before navigating
    // This prevents the loading screen from showing after setup completes
    await new Promise((resolve) => setTimeout(resolve, 100));
    onComplete();
    navigate("/dashboard", { replace: true });
  };

  return <SetupWizard onComplete={handleComplete} />;
};

/* ---------- App (routes) ---------- */

const App: React.FC = () => {
  const { user, loading: authLoading } = useAuth();
  const [setupNeeded, setSetupNeeded] = useState<boolean | null>(null);

  useEffect(() => {
    const checkSetup = async () => {
      try {
        console.log("[App] checking if setup is needed...");
        const setupRes = await axios.get("/api/setup-needed");
        console.log("[App] setup-needed response:", setupRes.data);
        setSetupNeeded(setupRes.data.needed);
      } catch (err) {
        console.error("[App] failed to check setup", err);
        setSetupNeeded(false);
      }
    };

    // Only check setup if auth has finished loading
    if (!authLoading) {
      console.log("[App] auth loading finished, user:", user);
      checkSetup();
    }
  }, [authLoading, user]);

  // Show loading while auth context is checking or setup status unknown
  if (authLoading || setupNeeded === null) {
    console.log(
      "[App] showing loading screen, authLoading:",
      authLoading,
      "setupNeeded:",
      setupNeeded,
    );
    return (
      <div className="flex items-center justify-center h-screen bg-surface text-white">
        Loading…
      </div>
    );
  }

  // If setup is needed, show setup wizard (takes priority over auth)
  if (setupNeeded) {
    console.log("[App] setup needed, showing wizard");
    return <SetupWizardWrapper onComplete={() => setSetupNeeded(false)} />;
  }

  // Otherwise show normal routes (ProtectedRoute will handle redirect to login if no user)
  console.log("[App] rendering routes, user:", user);
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/dashboard"
        element={
          <ProtectedRoute>
            <Layout>
              <DashboardPage />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/queries"
        element={
          <ProtectedRoute>
            <Layout>
              <QueriesPage />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/clients"
        element={
          <ProtectedRoute>
            <Layout>
              <ClientsPage />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/services"
        element={
          <ProtectedRoute>
            <Layout>
              <ServicesPage />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/ml"
        element={
          <ProtectedRoute>
            <Layout>
              <MLPage />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/settings"
        element={
          <ProtectedRoute>
            <Layout>
              <SettingsPage />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route path="/" element={<Navigate to="/login" replace />} />
    </Routes>
  );
};

export default App;
