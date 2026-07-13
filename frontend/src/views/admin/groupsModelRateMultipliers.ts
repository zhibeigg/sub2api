export const MAX_MODEL_RATE_MULTIPLIER_RULES = 100;

export interface ModelRateMultiplierRow {
  pattern: string;
  multiplier: number;
}

export type ModelRateMultiplierValidationCode =
  | "emptyPattern"
  | "invalidMultiplier"
  | "duplicatePattern"
  | "tooMany";

export class ModelRateMultiplierValidationError extends Error {
  constructor(
    public readonly code: ModelRateMultiplierValidationCode,
    public readonly details: { index?: number; pattern?: string; max?: number } = {},
  ) {
    super(code);
    this.name = "ModelRateMultiplierValidationError";
  }
}

const normalizePattern = (pattern: string) => pattern.trim().toLowerCase();

const comparePatterns = (left: string, right: string) => {
  if (left < right) return -1;
  if (left > right) return 1;
  return 0;
};

export const modelRateMultipliersToRows = (
  multipliers?: Record<string, number> | null,
): ModelRateMultiplierRow[] =>
  Object.entries(multipliers ?? {})
    .map(([pattern, multiplier], index) => ({
      pattern: normalizePattern(pattern),
      multiplier,
      index,
    }))
    .sort(
      (left, right) =>
        comparePatterns(left.pattern, right.pattern) || left.index - right.index,
    )
    .map(({ pattern, multiplier }) => ({ pattern, multiplier }));

export const modelRateMultiplierRowsToMap = (
  rows: ModelRateMultiplierRow[],
): Record<string, number> => {
  if (rows.length > MAX_MODEL_RATE_MULTIPLIER_RULES) {
    throw new ModelRateMultiplierValidationError("tooMany", {
      max: MAX_MODEL_RATE_MULTIPLIER_RULES,
    });
  }

  const normalizedRows = rows.map((row, index) => {
    const pattern = normalizePattern(row.pattern);
    if (!pattern) {
      throw new ModelRateMultiplierValidationError("emptyPattern", {
        index: index + 1,
      });
    }

    const multiplier = Number(row.multiplier);
    if (!Number.isFinite(multiplier) || multiplier <= 0) {
      throw new ModelRateMultiplierValidationError("invalidMultiplier", {
        index: index + 1,
        pattern,
      });
    }

    return { pattern, multiplier, index };
  });

  const seenPatterns = new Set<string>();
  for (const row of normalizedRows) {
    if (seenPatterns.has(row.pattern)) {
      throw new ModelRateMultiplierValidationError("duplicatePattern", {
        index: row.index + 1,
        pattern: row.pattern,
      });
    }
    seenPatterns.add(row.pattern);
  }

  return normalizedRows
    .sort(
      (left, right) =>
        comparePatterns(left.pattern, right.pattern) || left.index - right.index,
    )
    .reduce<Record<string, number>>((result, row) => {
      result[row.pattern] = row.multiplier;
      return result;
    }, {});
};
