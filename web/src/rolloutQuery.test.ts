import { describe, expect, it } from "vitest";
import { parseRolloutQuery } from "./rolloutQuery";

describe("indexed rollout query", () => {
  it("parses dimensions, ranges, sort, and text", () => {
    expect(parseRolloutQuery("nightly checkpoint:ckpt-42 tool:browser pass:false reward>=-1 tokens<9000 cost<=0.5 sort:cost desc:true")).toEqual({
      params: { limit: 200, q: "nightly", checkpoint: "ckpt-42", tool: "browser", pass: false, reward_min: -1, tokens_max: 9000, cost_max: 0.5, sort: "cost", descending: true },
      diagnostics: [],
    });
  });

  it("fails closed on unsupported or malformed clauses", () => {
    expect(parseRolloutQuery("reward:1 tool>shell mystery:x cost<-1 pass:maybe").diagnostics).toEqual([
      "reward requires <, <=, >, or >=", "tool supports only ':' or '='", "unknown indexed field 'mystery'", "cost requires a non-negative number", "pass must be true or false",
    ]);
  });

  it("rejects inverted numeric ranges before calling the API", () => {
    expect(parseRolloutQuery("reward>=2 reward<=1 tokens>=20 tokens<10").diagnostics).toEqual([
      "reward minimum must not exceed maximum",
      "tokens minimum must not exceed maximum",
    ]);
  });
});
