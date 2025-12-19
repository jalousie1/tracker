"use client";

import { useState, useEffect, useCallback } from "react";
import { Trash2, Plus, RefreshCw, AlertCircle } from "lucide-react";

type Token = {
  id: number;
  token_masked: string;
  user_id: string;
  status: string;
  failure_count: number;
  last_used: string;
};

export function TokenList({ adminKey }: { adminKey: string }) {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [newToken, setNewToken] = useState("");
  const [newOwner, setNewOwner] = useState("");
  const [adding, setAdding] = useState(false);

  const fetchTokens = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const res = await fetch("/api/admin/tokens", {
        headers: { "X-Admin-Key": adminKey },
      });
      if (!res.ok) throw new Error("Failed to fetch tokens");
      const data = await res.json();
      setTokens(data.tokens || []);
    } catch {
      setError("Failed to load tokens. Check your admin key.");
    } finally {
      setLoading(false);
    }
  }, [adminKey]);

  useEffect(() => {
    fetchTokens();
  }, [fetchTokens]);

  const handleAddToken = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newToken || !newOwner) return;

    setAdding(true);
    try {
      const res = await fetch("/api/admin/tokens", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Admin-Key": adminKey,
        },
        body: JSON.stringify({ token: newToken, owner_user_id: newOwner }),
      });

      const data = await res.json();
      if (!res.ok) throw new Error(data.error?.message || "Failed to add token");

      setNewToken("");
      setNewOwner("");
      fetchTokens();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to add token";
      alert(msg);
    } finally {
      setAdding(false);
    }
  };

  const handleRemoveToken = async (id: number) => {
    if (!confirm("Are you sure you want to delete this token?")) return;

    try {
      const res = await fetch(`/api/admin/tokens/${id}`, {
        method: "DELETE",
        headers: { "X-Admin-Key": adminKey },
      });
      if (!res.ok) throw new Error("Failed to delete token");
      fetchTokens();
    } catch {
      alert("Failed to remove token");
    }
  };

  return (
    <div className="w-full max-w-4xl space-y-8">
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold text-neutral-200">Token Management</h2>
        <button
          onClick={fetchTokens}
          className="p-2 text-neutral-400 hover:text-white transition-colors"
        >
          <RefreshCw className={`w-5 h-5 ${loading ? "animate-spin" : ""}`} />
        </button>
      </div>

      {error && (
        <div className="bg-red-900/20 border border-red-800 text-red-400 px-4 py-3 rounded flex items-center gap-2">
          <AlertCircle className="w-5 h-5" />
          {error}
        </div>
      )}

      {/* Add Token Form */}
      <form onSubmit={handleAddToken} className="bg-neutral-900/50 p-6 rounded-lg border border-neutral-800 space-y-4">
        <h3 className="text-lg font-medium text-neutral-300">Add New Token</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <input
            type="text"
            value={newToken}
            onChange={(e) => setNewToken(e.target.value)}
            placeholder="Discord Token"
            className="px-4 py-2 bg-neutral-900 border border-neutral-800 rounded text-neutral-200 focus:outline-none focus:border-neutral-600"
          />
          <input
            type="text"
            value={newOwner}
            onChange={(e) => setNewOwner(e.target.value)}
            placeholder="Owner User ID"
            className="px-4 py-2 bg-neutral-900 border border-neutral-800 rounded text-neutral-200 focus:outline-none focus:border-neutral-600"
          />
        </div>
        <button
          type="submit"
          disabled={adding || !newToken || !newOwner}
          className="flex items-center gap-2 px-4 py-2 bg-neutral-100 text-neutral-900 font-medium rounded hover:bg-neutral-300 transition-colors disabled:opacity-50"
        >
          {adding ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Plus className="w-4 h-4" />}
          Add Token
        </button>
      </form>

      {/* Token List */}
      <div className="space-y-4">
        {tokens.map((token) => (
          <div
            key={token.id}
            className="flex flex-col md:flex-row justify-between items-start md:items-center p-4 bg-neutral-900/30 border border-neutral-800 rounded-lg gap-4"
          >
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <span className="font-mono text-sm text-neutral-400">{token.token_masked}</span>
                <span
                  className={`px-2 py-0.5 text-xs rounded-full ${
                    token.status === "active"
                      ? "bg-green-900/30 text-green-400"
                      : token.status === "banned"
                      ? "bg-red-900/30 text-red-400"
                      : "bg-yellow-900/30 text-yellow-400"
                  }`}
                >
                  {token.status}
                </span>
              </div>
              <div className="text-xs text-neutral-500">
                Owner: {token.user_id} • Failures: {token.failure_count} • Last used: {new Date(token.last_used).toLocaleString()}
              </div>
            </div>
            <button
              onClick={() => handleRemoveToken(token.id)}
              className="p-2 text-neutral-500 hover:text-red-400 transition-colors"
              title="Remove Token"
            >
              <Trash2 className="w-5 h-5" />
            </button>
          </div>
        ))}

        {!loading && tokens.length === 0 && (
          <div className="text-center py-12 text-neutral-500">No tokens found. Add one above.</div>
        )}
      </div>
    </div>
  );
}

