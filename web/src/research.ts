import type {
  AnalyzerFinding,
  Trajectory,
  TrajectoryArtifact,
  TrajectoryEvent,
  TrajectorySignal,
} from "./types";

export type SemanticProvenance = "source-native" | "adapter-derived" | "inferred";

export interface DerivedValue<T> {
  value: T;
  provenance: SemanticProvenance;
  source: string;
}

export type MessageRole = "system" | "developer" | "user" | "assistant" | "tool" | "environment";

export interface ResearchMessage {
  eventId: string;
  role?: DerivedValue<MessageRole>;
  content: unknown;
  contentSource: "input" | "output" | "content" | "data";
}

export interface ResearchTool {
  eventId: string;
  name?: DerivedValue<string>;
  call?: unknown;
  result?: unknown;
}

export type LandmarkCategory = "message" | "tool" | "error" | "grader" | "reward" | "artifact" | "context" | "environment" | "state" | "log" | "event";

export interface SemanticLandmark {
  eventId: string;
  sequence: number;
  category: LandmarkCategory;
  label: string;
  provenance: SemanticProvenance;
}

export interface TranscriptGroup {
  id: string;
  eventIds: string[];
  kind: "preamble" | "turn" | "events";
  anchorRole?: MessageRole;
  provenance: SemanticProvenance;
}

export interface TokenTotals {
  total: number;
  provenance: SemanticProvenance;
  source: "signal" | "events";
  eventTotal?: number;
  countedEventIds: string[];
}

export interface GraderSummary {
  eventId: string;
  verdict?: unknown;
  score?: unknown;
  reason?: unknown;
  evidenceEventIds: string[];
}

export interface RewardSummary {
  total?: DerivedValue<number>;
  components: Record<string, number>;
  eventIds: string[];
}

export interface OutcomeSummary {
  status?: string;
  termination?: string;
  pass?: DerivedValue<boolean>;
  finalOutput?: ResearchMessage;
  finalOutputSelection?: DerivedValue<string>;
  graders: GraderSummary[];
  reward: RewardSummary;
  errorEventIds: string[];
}

const roles = new Set<MessageRole>(["system", "developer", "user", "assistant", "tool", "environment"]);

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function finiteNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function explicitMessage(value: unknown): { role?: MessageRole; content?: unknown } | undefined {
  const object = record(value);
  if (!object) return undefined;
  const role = typeof object.role === "string" && roles.has(object.role as MessageRole) ? object.role as MessageRole : undefined;
  if (role === undefined && !("content" in object)) return undefined;
  return { role, content: object.content };
}

/** Derive only display semantics supported by the canonical event envelope. */
export function deriveMessage(event: TrajectoryEvent): ResearchMessage | undefined {
  for (const [source, value] of [["input", event.input], ["output", event.output], ["content", event.content], ["data", event.data]] as const) {
    const message = explicitMessage(value);
    if (message && (message.role !== undefined || ((event.kind === "message" || event.kind === "generation") && message.content !== undefined))) {
      return {
        eventId: event.id,
        role: message.role ? { value: message.role, provenance: "source-native", source: `${source}.role` } : undefined,
        content: message.content,
        contentSource: source,
      };
    }
  }

  if (event.kind !== "message" && event.kind !== "generation") return undefined;
  const source = event.content !== undefined ? "content" : event.output !== undefined ? "output" : event.input !== undefined ? "input" : event.data !== undefined ? "data" : undefined;
  if (!source) return undefined;
  return {
    eventId: event.id,
    role: event.kind === "generation" ? { value: "assistant", provenance: "inferred", source: "event.kind=generation" } : undefined,
    content: event[source],
    contentSource: source,
  };
}

export function deriveTool(event: TrajectoryEvent): ResearchTool | undefined {
  if (event.kind !== "tool") return undefined;
  const input = record(event.input);
  const explicitName = typeof input?.name === "string" && input.name.trim() ? input.name : undefined;
  const titleName = typeof event.title === "string" && event.title.trim() ? event.title : undefined;
  return {
    eventId: event.id,
    name: explicitName
      ? { value: explicitName, provenance: "source-native", source: "input.name" }
      : titleName ? { value: titleName, provenance: "inferred", source: "event.title" } : undefined,
    call: input && "arguments" in input ? input.arguments : event.input,
    result: event.output,
  };
}

export function isContextEvent(event: TrajectoryEvent): boolean {
  return event.context !== undefined || event.alignment_key?.startsWith("context:") === true;
}

const contextOperationLabels = {
  compaction: "Context compacted",
  truncation: "Context truncated",
  injection: "Context injected",
  restore: "Context restored",
} as const;

function contextLandmarkProvenance(event: TrajectoryEvent): SemanticProvenance {
  if (event.context?.provenance === "source_native") return "source-native";
  if (event.context?.provenance === "adapter_derived") return "adapter-derived";
  if (event.title?.trim() || event.summary?.trim()) return "source-native";
  return "inferred";
}

function defaultLandmarkLabel(event: TrajectoryEvent, category: LandmarkCategory): string {
  if (event.context) return event.context.operation ? contextOperationLabels[event.context.operation] : "Context observation";
  const message = deriveMessage(event);
  const tool = deriveTool(event);
  if (tool?.name) return tool.name.value;
  if (message?.role) return `${message.role.value} message`;
  return category === "event" ? event.kind : category;
}

export function deriveLandmark(event: TrajectoryEvent): SemanticLandmark {
  const category: LandmarkCategory =
    isContextEvent(event) ? "context"
      : event.kind === "message" || event.kind === "generation" ? "message"
      : event.kind === "tool" ? "tool"
        : event.kind === "error" ? "error"
          : event.kind === "grader" ? "grader"
            : event.kind === "reward" ? "reward"
              : event.kind === "artifact" ? "artifact"
                : event.kind === "environment_action" || event.kind === "observation" ? "environment"
                  : event.kind === "state" ? "state"
                    : event.kind === "log" ? "log" : "event";
  const nativeLabel = event.title?.trim() || event.summary?.trim();
  return {
    eventId: event.id,
    sequence: event.sequence,
    category,
    label: nativeLabel || defaultLandmarkLabel(event, category),
    provenance: category === "context" ? contextLandmarkProvenance(event) : nativeLabel ? "source-native" : "inferred",
  };
}

function stableEvents(events: TrajectoryEvent[]): TrajectoryEvent[] {
  return events.map((event, index) => ({ event, index })).sort((left, right) => left.event.sequence - right.event.sequence || left.index - right.index).map(({ event }) => event);
}

/**
 * Groups the ordered stream around explicit user messages. A turn is deliberately
 * marked inferred: v1alpha1 has no canonical turn identity.
 */
export function groupTranscript(events: TrajectoryEvent[]): TranscriptGroup[] {
  const ordered = stableEvents(events);
  const groups: TranscriptGroup[] = [];
  let current: TranscriptGroup | undefined;
  for (const event of ordered) {
    const role = deriveMessage(event)?.role?.value;
    if (role === "user") {
      current = { id: `turn:${event.id}`, eventIds: [event.id], kind: "turn", anchorRole: "user", provenance: "inferred" };
      groups.push(current);
    } else if (current) {
      current.eventIds.push(event.id);
    } else {
      const previous = groups.at(-1);
      if (previous?.kind === "preamble") previous.eventIds.push(event.id);
      else groups.push({ id: `preamble:${event.id}`, eventIds: [event.id], kind: "preamble", anchorRole: role, provenance: "inferred" });
    }
  }
  return groups;
}

export function deriveTokenTotals(events: TrajectoryEvent[], signals: TrajectorySignal[] = []): TokenTotals | undefined {
  const counted: string[] = [];
  let eventTotal = 0;
  for (const event of events) {
    const value = finiteNumber(event.token_count) ?? finiteNumber(event.metadata?.tokens);
    if (value === undefined) continue;
    eventTotal += value;
    counted.push(event.id);
  }
  const reported = signals.find((signal) => ["token_count", "total_tokens", "tokens"].includes(signal.name) && finiteNumber(signal.value) !== undefined);
  if (reported) return { total: reported.value as number, provenance: "source-native", source: "signal", eventTotal: counted.length ? eventTotal : undefined, countedEventIds: counted };
  if (counted.length) return { total: eventTotal, provenance: "inferred", source: "events", countedEventIds: counted };
  return undefined;
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

export function deriveOutcome(trajectory: Trajectory, signals: TrajectorySignal[] = []): OutcomeSummary {
  const ordered = stableEvents(trajectory.events);
  const graders = ordered.filter((event) => event.kind === "grader").map((event): GraderSummary => {
    const output = record(event.output) ?? record(event.content) ?? record(event.data);
    return { eventId: event.id, verdict: output?.verdict, score: output?.score, reason: output?.reason ?? output?.feedback, evidenceEventIds: stringArray(output?.evidence) };
  });
  const rewardEvents = ordered.filter((event) => event.kind === "reward");
  const components: Record<string, number> = {};
  let eventReward: number | undefined;
  for (const event of rewardEvents) {
    const payload = record(event.data) ?? record(event.content) ?? record(event.output);
    eventReward = finiteNumber(event.reward) ?? finiteNumber(payload?.total) ?? finiteNumber(payload?.value) ?? eventReward;
    const values = record(payload?.components);
    if (values) for (const [name, value] of Object.entries(values)) if (finiteNumber(value) !== undefined) components[name] = value as number;
  }
  for (const signal of signals) {
    if (signal.name.startsWith("reward.") && finiteNumber(signal.value) !== undefined) components[signal.name.slice(7)] = signal.value as number;
  }
  const rewardSignal = signals.find((signal) => signal.name === "reward" && finiteNumber(signal.value) !== undefined);
  const total = finiteNumber(trajectory.total_reward) !== undefined
    ? { value: trajectory.total_reward as number, provenance: "source-native" as const, source: "trajectory.total_reward" }
    : rewardSignal ? { value: rewardSignal.value as number, provenance: "source-native" as const, source: `signal:${rewardSignal.id ?? rewardSignal.name}` }
      : eventReward !== undefined ? { value: eventReward, provenance: "source-native" as const, source: "reward event" } : undefined;
  const passSignal = signals.find((signal) => signal.name === "pass" && typeof signal.value === "boolean");
  const graderPass = graders.find((grader) => grader.verdict === "pass" || grader.verdict === "fail");
  const pass = passSignal
    ? { value: passSignal.value as boolean, provenance: "source-native" as const, source: `signal:${passSignal.id ?? passSignal.name}` }
    : graderPass ? { value: graderPass.verdict === "pass", provenance: "inferred" as const, source: `grader:${graderPass.eventId}.verdict` } : undefined;
  const assistant = ordered.map(deriveMessage).filter((message): message is ResearchMessage => message?.role?.value === "assistant");
  let explicitFinal: TrajectoryEvent | undefined;
  for (let index = ordered.length - 1; index >= 0; index--) {
    if (ordered[index].alignment_key === "message:assistant-final") {
      explicitFinal = ordered[index];
      break;
    }
  }
  const finalOutput = (explicitFinal ? deriveMessage(explicitFinal) : undefined) ?? assistant.at(-1);
  const finalOutputSelection = finalOutput
    ? explicitFinal && finalOutput.eventId === explicitFinal.id
      ? { value: finalOutput.eventId, provenance: "source-native" as const, source: "event.alignment_key=message:assistant-final" }
      : { value: finalOutput.eventId, provenance: "inferred" as const, source: "last assistant message" }
    : undefined;
  return {
    status: trajectory.status,
    termination: trajectory.termination,
    pass,
    finalOutput,
    finalOutputSelection,
    graders,
    reward: { total, components, eventIds: rewardEvents.map((event) => event.id) },
    errorEventIds: ordered.filter((event) => event.kind === "error").map((event) => event.id),
  };
}

export function artifactsForEvent(artifacts: TrajectoryArtifact[], eventId?: string): TrajectoryArtifact[] {
  return artifacts.filter((artifact) => eventId === undefined ? artifact.event_id === undefined : artifact.event_id === eventId);
}

export function findingsForEvent(findings: AnalyzerFinding[], eventId: string): AnalyzerFinding[] {
  return findings.filter((finding) => finding.event_ids?.includes(eventId));
}

export function linkedEventIds(artifacts: TrajectoryArtifact[], findings: AnalyzerFinding[]): Set<string> {
  const ids = new Set<string>();
  for (const artifact of artifacts) if (artifact.event_id) ids.add(artifact.event_id);
  for (const finding of findings) for (const eventId of finding.event_ids ?? []) ids.add(eventId);
  return ids;
}
