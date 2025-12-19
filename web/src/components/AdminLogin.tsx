"use client";

import { useState } from "react";

export function AdminLogin({
  onLogin,
  error,
  loading,
}: {
  onLogin: (key: string) => Promise<void>;
  error?: string;
  loading?: boolean;
}) {
  const [key, setKey] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (key.trim()) {
      onLogin(key.trim());
    }
  };

  return (
    <div className="flex flex-col items-center justify-center min-h-[50vh] space-y-6">
      <h2 className="text-2xl font-bold text-neutral-200">Admin Access</h2>
      <form onSubmit={handleSubmit} className="w-full max-w-sm space-y-4">
        <input
          type="password"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="Enter Admin Key"
          className="w-full px-4 py-2 bg-neutral-900 border border-neutral-800 rounded-md text-neutral-200 focus:outline-none focus:border-neutral-600"
        />
        {error ? <div className="text-sm text-red-400">{error}</div> : null}
        <button
          type="submit"
          disabled={!!loading}
          className="w-full px-4 py-2 bg-neutral-100 text-neutral-900 font-medium rounded-md hover:bg-neutral-300 transition-colors"
        >
          {loading ? "Validando..." : "Access Panel"}
        </button>
      </form>
    </div>
  );
}

