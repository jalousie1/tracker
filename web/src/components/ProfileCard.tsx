"use client";

import { useMemo, useState } from "react";
import DOMPurify from "dompurify";
import { BadgeCheck, LayoutGrid, Mic, Users, History, MessageSquare, Phone, Gamepad2, Palette } from "lucide-react";
import { FormattedDate } from "@/components/FormattedDate";
import { OverviewTab } from "./profile/OverviewTab";
import { GuildsTab } from "./profile/GuildsTab";
import { VoiceTab } from "./profile/VoiceTab";
import { MessagesTab } from "./profile/MessagesTab";

interface ProfileCardProps {
  user: {
    id: string;
    status: string;
    createdAt: string;
    displayName: string;
    username: string | null;
    globalName: string | null;
    avatarUrl: string | null;
    bioText: string | null;
  };
  currentVoice: {
    channelName: string;
    guildName: string;
    joinedAt: string;
  } | null;
  lastPresence: {
    status: string;
    changedAt: string;
    guildId: string | null;
  } | null;
  lastActivity: {
    name: string;
    startedAt: string;
    endedAt: string | null;
    details: string | null;
    state: string | null;
    type: number;
  } | null;
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
  voiceHistory: Array<{ id: string; channelName: string; guildName: string; joinedAt: string; leftAt: string | null; duration: string; wasVideo: boolean; wasStreaming: boolean; wasMuted: boolean; wasDeafened: boolean }>;
  messages: Array<{ id: string; content: string; guildName: string; channelName: string; createdAt: string }>;
  // Outros campos omitidos por brevidade no exemplo modular
  [key: string]: any;
}

function escapeHtml(s: string) {
  return s
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("\"", "&quot;")
    .replaceAll("'", "&#039;");
}

export function ProfileCard({
  user, currentVoice, lastPresence, lastActivity, timeline, connections,
  possibleAltIds, guilds, voiceHistory, messages
}: ProfileCardProps) {
  const [activeTab, setActiveTab] = useState<"overview" | "guilds" | "voice" | "messages" | "activity" | "cosmetics">("overview");

  const sanitizedBio = useMemo(() => {
    if (!user.bioText) return "";
    const safeTextAsHtml = escapeHtml(user.bioText).replaceAll("\n", "<br/>");
    return DOMPurify.sanitize(safeTextAsHtml, {
      ALLOWED_TAGS: ["br", "b", "strong", "i", "em", "u", "p", "span", "code", "a"],
      ALLOWED_ATTR: ["href", "target", "rel"],
    });
  }, [user.bioText]);

  const tabs = [
    { id: "overview", label: "Overview", icon: LayoutGrid },
    { id: "guilds", label: "Guilds", icon: Users },
    { id: "voice", label: "Voice", icon: Mic },
    { id: "messages", label: "Messages", icon: MessageSquare },
    { id: "activity", label: "Activity", icon: Gamepad2 },
    { id: "cosmetics", label: "Cosmetics", icon: Palette },
  ] as const;

  return (
    <div className="w-full max-w-5xl mx-auto">
      <div className="glass rounded-3xl overflow-hidden shadow-2xl bg-neutral-950/40 min-h-[800px]">
        {/* Banner */}
        <div className="h-32 bg-gradient-to-r from-neutral-900 via-neutral-800 to-neutral-900 opacity-50 relative">
          <div className="absolute inset-0 bg-[url('https://grainy-gradients.vercel.app/noise.svg')] opacity-20"></div>
        </div>

        <div className="px-8 pb-8">
          {/* Header */}
          <div className="relative flex flex-col sm:flex-row items-end -mt-12 mb-8 gap-6">
            <div className="w-24 h-24 rounded-full p-1 glass bg-neutral-900/50">
              {user.avatarUrl ? (
                <img src={user.avatarUrl} alt={user.displayName} className="w-full h-full rounded-full object-cover" />
              ) : (
                <div className="w-full h-full rounded-full bg-neutral-900 flex items-center justify-center text-neutral-400 font-mono text-xl">
                  {user.displayName.slice(0, 2).toUpperCase()}
                </div>
              )}
            </div>

            <div className="flex-1 mb-2">
              <h1 className="text-2xl font-bold text-white tracking-tight flex items-center gap-2">
                {user.globalName || user.username || user.displayName}
                {user.status === "active" && <BadgeCheck className="w-5 h-5 text-blue-400" />}
              </h1>
              {user.username && <p className="text-neutral-400 font-medium text-sm">@{user.username}</p>}
              <p className="text-xs text-neutral-500 font-mono mt-0.5">ID: {user.id}</p>

              {currentVoice && (
                <div className="mt-2 inline-flex items-center gap-3 px-3 py-1.5 rounded-lg bg-green-500/10 border border-green-500/20">
                  <span className="relative flex h-2 w-2">
                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                    <span className="relative inline-flex rounded-full h-2 w-2 bg-green-500"></span>
                  </span>
                  <span className="text-[11px] text-green-400 font-medium">{currentVoice.channelName} • {currentVoice.guildName}</span>
                </div>
              )}
            </div>
          </div>

          {/* Tabs */}
          <div className="flex items-center gap-2 mb-8 border-b border-neutral-800 overflow-x-auto">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              const isActive = activeTab === tab.id;
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id as any)}
                  className={`flex items-center gap-2 px-4 py-3 text-sm font-medium transition-colors relative ${isActive ? "text-white" : "text-neutral-400 hover:text-neutral-200"}`}
                >
                  <Icon className="w-4 h-4" />
                  {tab.label}
                  {isActive && <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-500 shadow-[0_0_10px_rgba(59,130,246,0.5)]" />}
                </button>
              );
            })}
          </div>

          {/* Content */}
          <div className="min-h-[400px]">
            {activeTab === "overview" && (
              <OverviewTab
                user={user}
                sanitizedBio={sanitizedBio}
                connections={connections}
                possibleAltIds={possibleAltIds}
                timeline={timeline}
              />
            )}
            {activeTab === "guilds" && <GuildsTab guilds={guilds} />}
            {activeTab === "voice" && <VoiceTab voiceHistory={voiceHistory} />}
            {activeTab === "messages" && <MessagesTab messages={messages} />}
            {["activity", "cosmetics"].includes(activeTab) && (
              <div className="py-20 text-center text-neutral-500">
                Tab "{activeTab}" is coming soon in the next modularization phase.
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
