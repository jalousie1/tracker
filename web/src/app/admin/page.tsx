"use client";

import { useState, useEffect } from "react";
import { AdminLogin } from "@/components/AdminLogin";
import { TokenList } from "@/components/TokenList";

export default function AdminPage() {
  const [adminKey, setAdminKey] = useState("");
  const [mounted, setMounted] = useState(false);
  const [authError, setAuthError] = useState("");
  const [authLoading, setAuthLoading] = useState(false);

  useEffect(() => {
    setMounted(true);
    const stored = localStorage.getItem("admin_key");
    if (stored) {
      // validar key salva antes de liberar o painel
      (async () => {
        setAuthLoading(true);
        setAuthError("");
        try {
          const res = await fetch("/api/admin/tokens", { headers: { "X-Admin-Key": stored } });
          if (!res.ok) throw new Error("invalid");
          setAdminKey(stored);
        } catch {
          localStorage.removeItem("admin_key");
          setAdminKey("");
          setAuthError("senha/admin key invalida");
        } finally {
          setAuthLoading(false);
        }
      })();
    }
  }, []);

  const handleLogin = async (key: string) => {
    setAuthLoading(true);
    setAuthError("");
    try {
      const res = await fetch("/api/admin/tokens", { headers: { "X-Admin-Key": key } });
      if (!res.ok) {
        setAuthError("senha/admin key invalida");
        return;
      }
      localStorage.setItem("admin_key", key);
      setAdminKey(key);
    } finally {
      setAuthLoading(false);
    }
  };

  const handleLogout = () => {
    localStorage.removeItem("admin_key");
    setAdminKey("");
    setAuthError("");
  };

  if (!mounted) return null;

  return (
    <div className="flex flex-col items-center min-h-screen w-full px-4 py-12 space-y-8">
      <div className="w-full max-w-4xl flex justify-between items-center">
        <h1 className="text-3xl font-bold text-neutral-100">Admin Panel</h1>
        {adminKey && (
          <button
            onClick={handleLogout}
            className="text-sm text-neutral-500 hover:text-neutral-300"
          >
            Logout
          </button>
        )}
      </div>

      {!adminKey ? (
        <AdminLogin onLogin={handleLogin} error={authError} loading={authLoading} />
      ) : (
        <TokenList adminKey={adminKey} />
      )}
    </div>
  );
}

