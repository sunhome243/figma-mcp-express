// Presence roster (PoC) — maps an `origin` label to a display identity (name +
// color + avatar) for the multi-agent live-highlight panel.
//
// The keys here MUST stay in sync with `rosterOrigins` in internal/tools.go (the
// Go enum that constrains the `origin` batch param). Real human names so an agent
// reads like a teammate; swap these for your actual team members by editing the
// keys/names in both places.
//
// Avatars are hot-linked from DiceBear (the plugin manifest allows all domains).
// They are deterministic per seed, so each name always renders the same face.
// When the image fails to load (offline), the UI falls back to a colored monogram
// (see initialOf + the <img on:error> path in App.svelte).

export interface AgentMeta {
  name: string;
  color: string;
  avatar: string;
}

const seedAvatar = (seed: string): string =>
  `https://api.dicebear.com/9.x/avataaars/svg?seed=${encodeURIComponent(seed)}&radius=50`;

export const ROSTER: Record<string, AgentMeta> = {
  grace: { name: "Grace", color: "#f43f5e", avatar: seedAvatar("Grace") },
  theo: { name: "Theo", color: "#f59e0b", avatar: seedAvatar("Theo") },
  sunho: { name: "Sunho", color: "#8b5cf6", avatar: seedAvatar("Sunho") },
  zoe: { name: "Zoe", color: "#3b82f6", avatar: seedAvatar("Zoe") },
  taewon: { name: "Taewon", color: "#14b8a6", avatar: seedAvatar("Taewon") },
  emma: { name: "Emma", color: "#06b6d4", avatar: seedAvatar("Emma") },
  alex: { name: "Alex", color: "#22c55e", avatar: seedAvatar("Alex") },
  rick: { name: "Rick", color: "#f97316", avatar: seedAvatar("Rick") },
};

const GENERIC: AgentMeta = {
  name: "Agent",
  color: "#9ca3af",
  avatar: seedAvatar("agent"),
};

// metaFor resolves an origin to its identity, falling back to a generic gray
// agent (labeled with the raw origin) for any value not in the roster.
export function metaFor(origin: string): AgentMeta {
  return ROSTER[origin] ?? { ...GENERIC, name: origin || "Agent" };
}

// initialOf is the monogram shown when the avatar image cannot load.
export function initialOf(origin: string): string {
  return (metaFor(origin).name[0] ?? "?").toUpperCase();
}
