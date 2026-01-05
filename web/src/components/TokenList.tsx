"use client";

import { useState, useEffect, useCallback } from "react";
import { Trash2, Plus, RefreshCw, AlertCircle, ChevronDown, ChevronUp, Users, Server } from "lucide-react";

type Guild = {
  guild_id: string;
  name: string;
  icon?: string;
  member_count?: number;
};

type Token = {
  id: number;
  token_masked: string;
  user_id: string;
  username?: string;
  display_name?: string;
  avatar?: string;
  status: string;
  failure_count: number;
  last_used: string;
  guilds?: Guild[];
};

export function TokenList({ adminKey }: { adminKey: string }) {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [newToken, setNewToken] = useState("");
  const [adding, setAdding] = useState(false);
  const [expandedTokens, setExpandedTokens] = useState<Set<number>>(new Set());

  const toggleExpanded = (tokenId: number) => {
    setExpandedTokens((prev) => {
      const next = new Set(prev);
      if (next.has(tokenId)) {
        next.delete(tokenId);
      } else {
        next.add(tokenId);
      }
      return next;
    });
  };

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
    if (!newToken) return;

    setAdding(true);
    try {
      const res = await fetch("/api/admin/tokens", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Admin-Key": adminKey,
        },
        body: JSON.stringify({ token: newToken }),
      });

      const data = await res.json();
      if (!res.ok) throw new Error(data.error?.message || "Failed to add token");

      setNewToken("");
      fetchTokens();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to add token";
      alert(msg);
    } finally {
      setAdding(false);
    }
  };

  const handleRemoveToken = async (id: number) => {
    if (!confirm("Tem certeza que deseja deletar este token?")) return;

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

  const getAvatarUrl = (userId: string, avatar?: string) => {
    if (!avatar) return null;
    return `https://cdn.discordapp.com/avatars/${userId}/${avatar}.png?size=64`;
  };

  const getGuildIconUrl = (guildId: string, icon?: string) => {
    if (!icon) return null;
    return `https://cdn.discordapp.com/icons/${guildId}/${icon}.png?size=32`;
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

      {/* Add Token Form - simplificado */}
      <form onSubmit={handleAddToken} className="bg-neutral-900/50 p-6 rounded-lg border border-neutral-800 space-y-4">
        <h3 className="text-lg font-medium text-neutral-300">Adicionar Token</h3>
        <div className="flex gap-4">
          <input
            type="text"
            value={newToken}
            onChange={(e) => setNewToken(e.target.value)}
            placeholder="Cole o token Discord aqui..."
            className="flex-1 px-4 py-2 bg-neutral-900 border border-neutral-800 rounded text-neutral-200 focus:outline-none focus:border-neutral-600 font-mono text-sm"
          />
          <button
            type="submit"
            disabled={adding || !newToken}
            className="flex items-center gap-2 px-4 py-2 bg-neutral-100 text-neutral-900 font-medium rounded hover:bg-neutral-300 transition-colors disabled:opacity-50"
          >
            {adding ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Plus className="w-4 h-4" />}
            Adicionar
          </button>
        </div>
        <p className="text-xs text-neutral-500">O username e servidores serão detectados automaticamente.</p>
      </form>

      {/* Token List */}
      <div className="space-y-3">
        {tokens.map((token) => {
          const isExpanded = expandedTokens.has(token.id);
          const hasGuilds = token.guilds && token.guilds.length > 0;

          return (
            <div
              key={token.id}
              className="bg-neutral-900/30 border border-neutral-800 rounded-lg overflow-hidden"
            >
              {/* Token Header */}
              <div
                className="flex flex-col md:flex-row justify-between items-start md:items-center p-4 gap-4 cursor-pointer hover:bg-neutral-800/30"
                onClick={() => toggleExpanded(token.id)}
              >
                <div className="flex items-center gap-3 flex-1">
                  {/* Avatar */}
                  {token.avatar ? (
                    <img
                      src={getAvatarUrl(token.user_id, token.avatar)!}
                      alt=""
                      className="w-10 h-10 rounded-full"
                    />
                  ) : (
                    <div className="w-10 h-10 rounded-full bg-neutral-700 flex items-center justify-center text-neutral-400">
                      {(token.username || "?").charAt(0).toUpperCase()}
                    </div>
                  )}

                  <div className="flex-1 min-w-0">
                    {/* Username & Display Name */}
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="font-medium text-neutral-100">
                        {token.display_name || token.username || "Conta Desconhecida"}
                      </span>
                      {token.username && (
                        <span className="text-sm text-neutral-500">@{token.username}</span>
                      )}
                      <span
                        className={`px-2 py-0.5 text-xs rounded-full ${token.status === "ativo"
                            ? "bg-green-900/30 text-green-400"
                            : token.status === "banido"
                              ? "bg-red-900/30 text-red-400"
                              : "bg-yellow-900/30 text-yellow-400"
                          }`}
                      >
                        {token.status}
                      </span>
                    </div>

                    {/* Info row */}
                    <div className="flex items-center gap-3 text-xs text-neutral-500 mt-1">
                      <span className="font-mono">{token.token_masked}</span>
                      {hasGuilds && (
                        <span className="flex items-center gap-1">
                          <Server className="w-3 h-3" />
                          {token.guilds!.length} servers
                        </span>
                      )}
                      <span>Failures: {token.failure_count}</span>
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  <button className="p-2 text-neutral-500">
                    {isExpanded ? <ChevronUp className="w-5 h-5" /> : <ChevronDown className="w-5 h-5" />}
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleRemoveToken(token.id);
                    }}
                    className="p-2 text-neutral-500 hover:text-red-400 transition-colors"
                    title="Remover Token"
                  >
                    <Trash2 className="w-5 h-5" />
                  </button>
                </div>
              </div>

              {/* Expanded Details - Accordion */}
              {isExpanded && (
                <div className="border-t border-neutral-800 px-4 py-3 bg-neutral-950/50">
                  {hasGuilds ? (
                    <div className="space-y-2">
                      <h4 className="text-sm font-medium text-neutral-400 flex items-center gap-2">
                        <Server className="w-4 h-4" />
                        Servidores ({token.guilds!.length})
                      </h4>
                      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
                        {token.guilds!.map((guild) => (
                          <div
                            key={guild.guild_id}
                            className="flex items-center gap-2 p-2 bg-neutral-900/50 rounded border border-neutral-800"
                          >
                            {guild.icon ? (
                              <img
                                src={getGuildIconUrl(guild.guild_id, guild.icon)!}
                                alt=""
                                className="w-8 h-8 rounded-full"
                              />
                            ) : (
                              <div className="w-8 h-8 rounded-full bg-neutral-700 flex items-center justify-center text-xs text-neutral-400">
                                {guild.name.charAt(0).toUpperCase()}
                              </div>
                            )}
                            <div className="flex-1 min-w-0">
                              <div className="text-sm text-neutral-200 truncate">{guild.name}</div>
                              {guild.member_count && (
                                <div className="text-xs text-neutral-500 flex items-center gap-1">
                                  <Users className="w-3 h-3" />
                                  {guild.member_count.toLocaleString()}
                                </div>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : (
                    <div className="text-sm text-neutral-500 text-center py-4">
                      Nenhum servidor encontrado. Inicie o gateway para sincronizar.
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}

        {!loading && tokens.length === 0 && (
          <div className="text-center py-12 text-neutral-500">Nenhum token encontrado. Adicione um acima.</div>
        )}
      </div>
    </div>
  );
}
