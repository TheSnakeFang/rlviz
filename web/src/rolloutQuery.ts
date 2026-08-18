import type { RolloutQueryParams } from "./types";

export interface ParsedRolloutQuery {
  params: RolloutQueryParams;
  diagnostics: string[];
}

const structured = /^([A-Za-z][\w.-]*)(:|=|<=|>=|<|>)(.*)$/;
const exactFields: Record<string, keyof RolloutQueryParams> = {
  source: "source", run: "run", case: "case", task: "case", group: "group", checkpoint: "checkpoint", model: "model",
  environment: "environment_version", env: "environment_version", status: "status", termination: "termination", tool: "tool",
};
const numericFields: Record<string, { min: keyof RolloutQueryParams; max: keyof RolloutQueryParams; nonnegative: boolean }> = {
  reward: { min: "reward_min", max: "reward_max", nonnegative: false },
  tokens: { min: "tokens_min", max: "tokens_max", nonnegative: true },
  cost: { min: "cost_min", max: "cost_max", nonnegative: true },
};
const sorts = new Set(["reward", "tokens", "cost", "tools", "run", "case", "checkpoint", "model"]);

/** Parse the collection rail's bounded whitespace-separated indexed query. */
export function parseRolloutQuery(source: string): ParsedRolloutQuery {
  const params: RolloutQueryParams = { limit: 200 };
  const diagnostics: string[] = [];
  const text: string[] = [];
  for (const token of source.match(/\S+/g) ?? []) {
    const match = structured.exec(token);
    if (!match) { text.push(token); continue; }
    const [, rawField, operator, rawValue] = match;
    const field = rawField.toLowerCase();
    if (!rawValue) { diagnostics.push(`${rawField} requires a value`); continue; }
    const exact = exactFields[field];
    if (exact) {
      if (operator !== ":" && operator !== "=") diagnostics.push(`${rawField} supports only ':' or '='`);
      else (params as Record<string, unknown>)[exact] = rawValue;
      continue;
    }
    if (field === "pass") {
      if ((operator !== ":" && operator !== "=") || !["true", "false"].includes(rawValue.toLowerCase())) diagnostics.push("pass must be true or false");
      else params.pass = rawValue.toLowerCase() === "true";
      continue;
    }
    if (field === "sort") {
      if ((operator !== ":" && operator !== "=") || !sorts.has(rawValue)) diagnostics.push(`unknown sort '${rawValue}'`);
      else params.sort = rawValue as RolloutQueryParams["sort"];
      continue;
    }
    if (field === "desc" || field === "descending") {
      if ((operator !== ":" && operator !== "=") || !["true", "false"].includes(rawValue.toLowerCase())) diagnostics.push("descending must be true or false");
      else params.descending = rawValue.toLowerCase() === "true";
      continue;
    }
    const numeric = numericFields[field];
    if (numeric) {
      const value = Number(rawValue);
      if (!Number.isFinite(value) || (numeric.nonnegative && value < 0)) { diagnostics.push(`${rawField} requires a ${numeric.nonnegative ? "non-negative " : ""}number`); continue; }
      if (operator === ">" || operator === ">=") (params as Record<string, unknown>)[numeric.min] = value;
      else if (operator === "<" || operator === "<=") (params as Record<string, unknown>)[numeric.max] = value;
      else diagnostics.push(`${rawField} requires <, <=, >, or >=`);
      continue;
    }
    diagnostics.push(`unknown indexed field '${rawField}'`);
  }
  if (text.length) params.q = text.join(" ");
  for (const [label, minimum, maximum] of [
    ["reward", params.reward_min, params.reward_max],
    ["tokens", params.tokens_min, params.tokens_max],
    ["cost", params.cost_min, params.cost_max],
  ] as const) {
    if (minimum !== undefined && maximum !== undefined && minimum > maximum) diagnostics.push(`${label} minimum must not exceed maximum`);
  }
  return { params, diagnostics };
}
