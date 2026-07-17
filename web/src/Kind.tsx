const kindMark: Record<string, string> = {
  message: "M", generation: "AI", tool: "T", environment_action: "A", observation: "O",
  state: "S", reward: "R", grader: "G", artifact: "F", error: "!", log: "L",
};

export function Kind({ kind }: { kind: string }) {
  return <span className={`kind kind-${kind}`} aria-label={kind}>{kindMark[kind] || kind.slice(0, 2).toUpperCase()}</span>;
}
