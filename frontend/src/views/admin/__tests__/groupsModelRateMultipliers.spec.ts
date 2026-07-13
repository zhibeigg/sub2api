import { describe, expect, it } from "vitest";

import {
  MAX_MODEL_RATE_MULTIPLIER_RULES,
  ModelRateMultiplierValidationError,
  modelRateMultiplierRowsToMap,
  modelRateMultipliersToRows,
} from "../groupsModelRateMultipliers";

const expectValidationCode = (
  action: () => unknown,
  code: ModelRateMultiplierValidationError["code"],
) => {
  try {
    action();
    throw new Error("Expected validation error");
  } catch (error) {
    expect(error).toBeInstanceOf(ModelRateMultiplierValidationError);
    expect((error as ModelRateMultiplierValidationError).code).toBe(code);
  }
};

describe("groupsModelRateMultipliers", () => {
  it("normalizes and stably sorts map entries", () => {
    expect(
      modelRateMultipliersToRows({
        " GPT-4* ": 1.5,
        "claude-opus-*": 2,
        " gpt-4o ": 0.8,
      }),
    ).toEqual([
      { pattern: "claude-opus-*", multiplier: 2 },
      { pattern: "gpt-4*", multiplier: 1.5 },
      { pattern: "gpt-4o", multiplier: 0.8 },
    ]);
  });

  it("preserves source order when normalized patterns compare equally", () => {
    expect(
      modelRateMultipliersToRows({
        " GPT-4O ": 1,
        "gpt-4o": 2,
      }),
    ).toEqual([
      { pattern: "gpt-4o", multiplier: 1 },
      { pattern: "gpt-4o", multiplier: 2 },
    ]);
  });

  it("normalizes patterns and returns a sorted map", () => {
    expect(
      modelRateMultiplierRowsToMap([
        { pattern: " GPT-4O ", multiplier: 0.8 },
        { pattern: "Claude-Opus-*", multiplier: 2 },
      ]),
    ).toEqual({
      "claude-opus-*": 2,
      "gpt-4o": 0.8,
    });
  });

  it("rejects duplicate patterns after normalization", () => {
    expectValidationCode(
      () =>
        modelRateMultiplierRowsToMap([
          { pattern: "GPT-4O", multiplier: 1 },
          { pattern: " gpt-4o ", multiplier: 2 },
        ]),
      "duplicatePattern",
    );
  });

  it.each([0, -1, Number.NaN, Number.POSITIVE_INFINITY])(
    "rejects invalid multiplier %s",
    (multiplier) => {
      expectValidationCode(
        () => modelRateMultiplierRowsToMap([{ pattern: "gpt-*", multiplier }]),
        "invalidMultiplier",
      );
    },
  );

  it("rejects an empty normalized pattern", () => {
    expectValidationCode(
      () => modelRateMultiplierRowsToMap([{ pattern: "   ", multiplier: 1 }]),
      "emptyPattern",
    );
  });

  it("accepts 100 rules and rejects the 101st", () => {
    const rows = Array.from(
      { length: MAX_MODEL_RATE_MULTIPLIER_RULES },
      (_, index) => ({ pattern: `model-${index}`, multiplier: 1 }),
    );

    expect(Object.keys(modelRateMultiplierRowsToMap(rows))).toHaveLength(100);
    expectValidationCode(
      () =>
        modelRateMultiplierRowsToMap([
          ...rows,
          { pattern: "model-over-limit", multiplier: 1 },
        ]),
      "tooMany",
    );
  });
});
