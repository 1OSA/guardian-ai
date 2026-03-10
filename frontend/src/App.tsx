import React, { useEffect, useState } from "react";
import {
  BrowserRouter,
  Routes,
  Route,
  Navigate,
  useNavigate,
} from "react-router-dom";
import axios from "axios";

import { AuthProvider, ProtectedRoute } from "./lib/AuthContext";
import Layout from "./components/Layout";

import LoginPage from "./pages/LoginPage";
import DashboardPage from "./pages/DashboardPage";
import QueriesPage from "./pages/QueriesPage";
import ClientsPage from "./pages/ClientsPage";
import ServicesPage from "./pages/ServicesPage";
import MLPage from "./pages/MLPage";
import SettingsPage from "./pages/SettingsPage";
import SetupWizard from "./pages/SetupWizard";
import { useAuth } from "./lib/AuthContext";

/* ---------- Setup Wizard Wrapper (auto-login after setup) ---------- */

const SetupWizardWrapper: React.FC<{ onComplete: () => void }> = ({
  onComplete,
}) => {
  const { login } = useAuth();
  const navigate = useNavigate();

  const handleComplete = async (username: string, password: string) => {
    try {
      await login(username, password);
    } catch {
      // login failure is non-fatal — user can log in manually
    }
    onComplete();
    navigate("/dashboard", { replace: true });
  };

  return <SetupWizard onComplete={handleComplete} />;
};

/* ---------- App (routes) ---------- */

const App: React.FC = () => {
  const [setupNeeded, setSetupNeeded] = useState<boolean | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const checkSetup = async () => {
      try {
        const res = await axios.get("/api/setup-needed");
        setSetupNeeded(res.data.needed);
      } catch (err) {
        console.error("Failed to check setup", err);
        setSetupNeeded(false);
      } finally {
        setLoading(false);
      }
    };
    checkSetup();
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen bg-surface text-white">
        Loading…
      </div>
    );
  }

  if (setupNeeded) {
    return (
      <BrowserRouter>
        <AuthProvider>
          <SetupWizardWrapper onComplete={() => setSetupNeeded(false)} />
        </AuthProvider>
      </BrowserRouter>
    );
  }

  return (
    <BrowserRouter>
      <AuthProvider>
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
      </AuthProvider>
    </BrowserRouter>
  );
};

export default App;
