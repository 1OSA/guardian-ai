import React, { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../lib/AuthContext";

const LoginPage: React.FC = () => {
  const { login, loading, user } = useAuth();
  const navigate = useNavigate();
  const [username, setUsername] = useState<string>("");
  const [password, setPassword] = useState<string>("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (user) navigate("/dashboard");
  }, [user, navigate]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    try {
      await login(username, password);
      navigate("/dashboard");
    } catch (err: unknown) {
      setError(
        (err as { response?: { data?: string } })?.response?.data ||
          "Login failed",
      );
    }
  };

  return (
    <div className="min-h-screen bg-surface flex items-center justify-center p-5">
      <div className="bg-surface-1 p-10 rounded-xl shadow-lg max-w-sm w-full text-white">
        <h2 className="mb-5 text-center text-xl font-bold">Sign in</h2>
        {error && (
          <div className="text-red-400 mb-4 text-center text-sm">{error}</div>
        )}
        <form onSubmit={submit}>
          <div className="mb-4">
            <label className="block mb-1.5 font-semibold text-sm text-text">
              Username
            </label>
            <input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full px-3 py-3 border border-text-ghost rounded bg-surface text-white text-base outline-none box-border"
            />
          </div>
          <div className="mb-5">
            <label className="block mb-1.5 font-semibold text-sm text-text">
              Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-3 py-3 border border-text-ghost rounded bg-surface text-white text-base outline-none box-border"
            />
          </div>
          <button
            type="submit"
            disabled={loading}
            className="w-full py-3 bg-accent text-white border-none rounded text-base cursor-pointer transition-opacity hover:opacity-80 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? "Signing in…" : "Sign in"}
          </button>
        </form>
      </div>
    </div>
  );
};

export default LoginPage;
