"use client";

import { useMemo, useState } from "react";
import DOMPurify from "dompurify";
import { Timeline } from "@/components/Timeline";
import { Connections } from "@/components/Connections";
import { BadgeCheck, LayoutGrid, Mic, Users, User, History } from "lucide-react";

interface ProfileCardProps {
  user: {
    id: string;
    status: string;
    createdAt: string;
    displayName: string;
    avatarUrl: string | null;
    bioText: string | null;
  };
  timeline: {
    events: Array<{
      id: string;
      kind: "username" | "global_name" | "nickname" | "bio";
      at: string;
      from: string | null;
      to: string | null;
    }>;
    avatars: Array<{ id: string; url: string; at: string }>;
  };
  connections: Array<{ id: string; platform: string; label: string; url: string | null }>;
  possibleAltIds?: string[];
  guilds: Array<{ id: string; name: string; joinedAt: string; leftAt: string | null; iconUrl: string | null }>;
  voiceHistory: Array<{ id: string; channelName: string; guildName: string; joinedAt: string; leftAt: string | null; duration: string }>;
  nicknames: Array<{ id: string; nickname: string; guildName: string; observedAt: string }>;
}

function escapeHtml(s: string) {
  return s
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("\"", "&quot;")
    .replaceAll("'", "&#039;");
}

export function ProfileCard({ user, timeline, connections, possibleAltIds, guilds, voiceHistory, nicknames }: ProfileCardProps) {
  const [activeTab, setActiveTab] = useState<"overview" | "guilds" | "voice" | "nicknames">("overview");

  const sanitizedBio = useMemo(() => {
    if (!user.bioText) return "";

    // sanitize html (xss) - tratando a bio como texto e convertendo \n -> <br/>
    const safeTextAsHtml = escapeHtml(user.bioText).replaceAll("\n", "<br/>");
    return DOMPurify.sanitize(safeTextAsHtml, {
      ALLOWED_TAGS: ["br", "b", "strong", "i", "em", "u", "p", "span", "code", "a"],
      ALLOWED_ATTR: ["href", "target", "rel"],
    });
  }, [user.bioText]);

  const tabs = [
    { id: "overview", label: "Overview", icon: LayoutGrid },
    { id: "guilds", label: "Guilds", icon: Users },
    { id: "voice", label: "Voice History", icon: Mic },
    { id: "nicknames", label: "Nicknames", icon: History },
  ] as const;

  return (
    <div className="w-full max-w-5xl mx-auto">
      {/* Glass Card Container */}
      <div className="glass rounded-3xl overflow-hidden shadow-2xl bg-neutral-950/40 min-h-[800px]">
        
        {/* Banner/Header Background */}
        <div className="h-32 bg-gradient-to-r from-neutral-900 via-neutral-800 to-neutral-900 opacity-50 relative">
             <div className="absolute inset-0 bg-[url('https://grainy-gradients.vercel.app/noise.svg')] opacity-20"></div>
        </div>

        <div className="px-8 pb-8">
            {/* Header Profile Info */}
            <div className="relative flex flex-col sm:flex-row items-end sm:items-end -mt-12 mb-8 gap-6">
                <div className="relative">
                    <div className="w-24 h-24 rounded-full p-1 glass bg-neutral-900/50">
                        {user.avatarUrl ? (
                          <img
                            src={user.avatarUrl}
                            alt={user.displayName}
                            className="w-full h-full rounded-full object-cover"
                          />
                        ) : (
                          <div className="w-full h-full rounded-full bg-neutral-900 flex items-center justify-center text-neutral-400 font-mono text-xl">
                            {user.displayName.slice(0, 2).toUpperCase()}
                          </div>
                        )}
                    </div>
                </div>
                
                <div className="flex-1 mb-2">
                    <div className="flex items-center gap-2">
                        <h1 className="text-2xl font-bold text-white tracking-tight">{user.displayName}</h1>
                        {user.status === "active" && <BadgeCheck className="w-5 h-5 text-blue-400" />}
                    </div>
                    <p className="text-sm text-neutral-400 font-mono">ID: {user.id}</p>
                </div>
            </div>

            {/* Tabs Navigation */}
            <div className="flex items-center gap-2 mb-8 border-b border-neutral-800 overflow-x-auto">
              {tabs.map((tab) => {
                const Icon = tab.icon;
                const isActive = activeTab === tab.id;
                return (
                  <button
                    key={tab.id}
                    onClick={() => setActiveTab(tab.id)}
                    className={`
                      flex items-center gap-2 px-4 py-3 text-sm font-medium transition-colors relative
                      ${isActive ? "text-white" : "text-neutral-400 hover:text-neutral-200"}
                    `}
                  >
                    <Icon className="w-4 h-4" />
                    {tab.label}
                    {isActive && (
                      <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-500 shadow-[0_0_10px_rgba(59,130,246,0.5)]" />
                    )}
                  </button>
                );
              })}
            </div>

            {/* Tab Content */}
            <div className="min-h-[400px]">
              {activeTab === "overview" && (
                <div className="grid grid-cols-1 lg:grid-cols-3 gap-8 animate-in fade-in slide-in-from-bottom-4 duration-500">
                    {/* Left Column: Bio & Connections */}
                    <div className="space-y-8">
                        {/* Bio Section */}
                        {sanitizedBio && (
                            <div className="space-y-2">
                                <h3 className="text-sm font-medium text-neutral-400">bio</h3>
                                <div 
                                    className="text-sm text-neutral-300 leading-relaxed"
                                    dangerouslySetInnerHTML={{ __html: sanitizedBio }} 
                                />
                            </div>
                        )}

                        <Connections connections={connections} possibleAltIds={possibleAltIds} />
                    </div>

                    {/* Right Column: Timeline (Span 2) */}
                    <div className="lg:col-span-2">
                        <Timeline events={timeline.events} avatars={timeline.avatars} />
                    </div>
                </div>
              )}

              {activeTab === "guilds" && (
                <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
                  <h3 className="text-lg font-medium text-white mb-4">Guild History</h3>
                  {guilds.length === 0 ? (
                    <p className="text-neutral-500">No guild history recorded.</p>
                  ) : (
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      {guilds.map((guild) => (
                        <div key={guild.id} className="p-4 rounded-xl bg-neutral-900/50 border border-neutral-800 flex items-center gap-4">
                          {guild.iconUrl ? (
                            <img src={guild.iconUrl} alt={guild.name} className="w-12 h-12 rounded-full" />
                          ) : (
                            <div className="w-12 h-12 rounded-full bg-neutral-800 flex items-center justify-center text-neutral-500">
                              <Users className="w-6 h-6" />
                            </div>
                          )}
                          <div>
                            <div className="font-medium text-white">{guild.name}</div>
                            <div className="text-xs text-neutral-500 font-mono">ID: {guild.id}</div>
                            <div className="text-xs text-neutral-400 mt-1">
                              Joined: {new Date(guild.joinedAt).toLocaleDateString()}
                              {guild.leftAt && ` • Left: ${new Date(guild.leftAt).toLocaleDateString()}`}
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {activeTab === "voice" && (
                <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
                  <h3 className="text-lg font-medium text-white mb-4">Voice Channel History</h3>
                  {voiceHistory.length === 0 ? (
                    <p className="text-neutral-500">No voice history recorded.</p>
                  ) : (
                    <div className="space-y-2">
                      {voiceHistory.map((voice) => (
                        <div key={voice.id} className="p-3 rounded-lg bg-neutral-900/30 border border-neutral-800/50 flex items-center justify-between hover:bg-neutral-900/50 transition-colors">
                          <div className="flex items-center gap-3">
                            <div className="w-8 h-8 rounded-full bg-neutral-800 flex items-center justify-center text-neutral-500">
                              <Mic className="w-4 h-4" />
                            </div>
                            <div>
                              <div className="text-sm font-medium text-neutral-200">{voice.channelName}</div>
                              <div className="text-xs text-neutral-500">{voice.guildName}</div>
                            </div>
                          </div>
                          <div className="text-right">
                            <div className="text-sm font-mono text-blue-400">{voice.duration}</div>
                            <div className="text-xs text-neutral-500">
                              {new Date(voice.joinedAt).toLocaleString()}
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {activeTab === "nicknames" && (
                <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
                  <h3 className="text-lg font-medium text-white mb-4">Nickname History</h3>
                  {nicknames.length === 0 ? (
                    <p className="text-neutral-500">No nickname history recorded.</p>
                  ) : (
                    <div className="space-y-2">
                      {nicknames.map((nick) => (
                        <div key={nick.id} className="p-3 rounded-lg bg-neutral-900/30 border border-neutral-800/50 flex items-center justify-between">
                          <div className="flex items-center gap-3">
                            <div className="w-8 h-8 rounded-full bg-neutral-800 flex items-center justify-center text-neutral-500">
                              <User className="w-4 h-4" />
                            </div>
                            <div>
                              <div className="text-sm font-medium text-neutral-200">{nick.nickname}</div>
                              <div className="text-xs text-neutral-500">{nick.guildName}</div>
                            </div>
                          </div>
                          <div className="text-xs text-neutral-500 font-mono">
                            {new Date(nick.observedAt).toLocaleString()}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>
        </div>
      </div>
    </div>
  );
}

