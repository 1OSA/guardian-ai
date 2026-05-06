import React, { createContext, useContext, useEffect, useState } from "react";
import { Navigate } from "react-router-dom";
import axios from "axios";
import type { AuthContextType } from "./types";

const AuthContext = createContext<AuthContextType | undefined>(undefined);

// eslint-disable-next-line react-refresh/only-export-components
export const useAuth = (): AuthContextType => {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
};

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [user, setUser] = useState<string | null>(null);
  const [loading, setLoading] = useState<boolean>(true);

  const checkAuth = async () => {
    setLoading(true);
    try {
      const userRes = await axios.get("/api/user");
      setUser(userRes.data.username);
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    checkAuth();
  }, []);

  const login = async (username: string, password: string) => {
    setLoading(true);
    try {
      await axios.post("/api/auth/login", { username, password });
      const userRes = await axios.get("/api/user");
      setUser(userRes.data.username);
    } finally {
      setLoading(false);
    }
  };

  const logout = async () => {
    try {
      await axios.post("/api/auth/logout");
    } catch {
      // ignore error
    } finally {
      setUser(null);
    }
  };

  const refreshAuth = async () => {
    await checkAuth();
  };

  return (
    <AuthContext.Provider value={{ user, loading, login, logout, refreshAuth }}>
      {children}
    </AuthContext.Provider>
  );
};

export const ProtectedRoute: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const { user, loading } = useAuth();
  if (loading) return <div style={{ padding: 24 }}>Checking session…</div>;
  if (!user) return <Navigate to="/login" replace />;
  return <>{children}</>;
};
