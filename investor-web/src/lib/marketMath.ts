import type { MarketLayer, ObservationKind } from "@/data/types";

export interface LayerInputs {
  learners: number;
  annualRevenuePerLearner: number;
}

export interface LayerComputation extends LayerInputs {
  id: string;
  modeledCeiling: number;
  equation: string;
  sourceYear: string;
  observedOrModeled: ObservationKind;
  additive: false;
  overlapGroup: string;
  package: MarketLayer["package"];
  segment: string;
  caveat: string;
  dirty: boolean;
}

export interface CombineResult {
  allowed: false;
  reason: string;
  overlappingGroups: string[];
  layerIds: string[];
}

/**
 * Pure ceiling arithmetic: learners × annual ARPU.
 * Returns 0 for non-finite or negative inputs after clamping at 0.
 */
export function computeLayerCeiling(learners: number, annualArpu: number): number {
  const safeLearners = sanitizeNonNegative(learners);
  const safeArpu = sanitizeNonNegative(annualArpu);
  return safeLearners * safeArpu;
}

/** Build a copyable textual equation for a layer. */
export function formatLayerEquation(
  learners: number,
  annualArpu: number,
  ceiling?: number,
): string {
  const safeLearners = sanitizeNonNegative(learners);
  const safeArpu = sanitizeNonNegative(annualArpu);
  const value = ceiling ?? computeLayerCeiling(safeLearners, safeArpu);
  return `${formatCount(safeLearners)} learners × $${formatCount(safeArpu)}/yr = $${formatCount(value)}`;
}

/** Defaults captured from structured market layer records. */
export function defaultsFromLayer(layer: MarketLayer): LayerInputs {
  return {
    learners: layer.learners,
    annualRevenuePerLearner: layer.annualRevenuePerLearner,
  };
}

/** Whether inputs still match sourced defaults. */
export function isDefaultInputs(layer: MarketLayer, inputs: LayerInputs): boolean {
  return (
    inputs.learners === layer.learners &&
    inputs.annualRevenuePerLearner === layer.annualRevenuePerLearner
  );
}

/**
 * Recalculate a single expansion layer from editable fields while preserving
 * sourced metadata. Observation kind becomes "derived" once dirty.
 */
export function recalculateLayer(layer: MarketLayer, inputs: LayerInputs): LayerComputation {
  const learners = sanitizeNonNegative(inputs.learners);
  const annualRevenuePerLearner = sanitizeNonNegative(inputs.annualRevenuePerLearner);
  const modeledCeiling = computeLayerCeiling(learners, annualRevenuePerLearner);
  const dirty = !isDefaultInputs(layer, { learners, annualRevenuePerLearner });

  return {
    id: layer.id,
    learners,
    annualRevenuePerLearner,
    modeledCeiling,
    equation: formatLayerEquation(learners, annualRevenuePerLearner, modeledCeiling),
    sourceYear: layer.sourceYear,
    observedOrModeled: dirty ? "derived" : layer.observedOrModeled,
    additive: false,
    overlapGroup: layer.overlapGroup,
    package: layer.package,
    segment: layer.segment,
    caveat: layer.caveat,
    dirty,
  };
}

/**
 * Overlap prevention: never allow a combined total across layers that share
 * an overlap group, or across different packages in the family expansion set.
 * Institutional layers must stay separate from family layers.
 */
export function tryCombineLayerCeilings(
  layers: ReadonlyArray<Pick<LayerComputation, "id" | "overlapGroup" | "package" | "modeledCeiling" | "additive">>,
): CombineResult {
  const ids = layers.map((l) => l.id);
  if (layers.length <= 1) {
    return {
      allowed: false,
      reason: "Combined totals are disabled. Inspect each expansion layer independently.",
      overlappingGroups: unique(layers.map((l) => l.overlapGroup)),
      layerIds: ids,
    };
  }

  const additiveViolation = layers.some((l) => l.additive !== false);
  if (additiveViolation) {
    return {
      allowed: false,
      reason: "A layer marked additive slipped through; refuse the total.",
      overlappingGroups: unique(layers.map((l) => l.overlapGroup)),
      layerIds: ids,
    };
  }

  const groups = unique(layers.map((l) => l.overlapGroup));
  const packages = unique(layers.map((l) => l.package));
  const hasFamily = packages.some((p) => p === "base" || p === "core" || p === "premier");
  const hasInstitutional = packages.includes("institutional");

  if (hasFamily && hasInstitutional) {
    return {
      allowed: false,
      reason:
        "Family subscription layers and institutional contracts cannot be combined without an explicit contracting party and deduplication rule.",
      overlappingGroups: groups,
      layerIds: ids,
    };
  }

  // Any multi-layer set is refused: populations overlap or are alternative boundaries.
  const sharedGroups = groups.filter(
    (g) => layers.filter((l) => l.overlapGroup === g).length > 1,
  );

  return {
    allowed: false,
    reason:
      sharedGroups.length > 0
        ? `Overlapping populations share group(s): ${sharedGroups.join(", ")}. Do not sum.`
        : "Expansion layers are alternative ceilings, not additive segments. Combined totals stay disabled.",
    overlappingGroups: groups,
    layerIds: ids,
  };
}

/** Institutional scenario: targeted learners × annual price. */
export function computeInstitutionalProgramme(
  targetedLearners: number,
  annualPriceUsd: number,
): { learners: number; annualPriceUsd: number; annualValueUsd: number; equation: string } {
  const learners = sanitizeNonNegative(targetedLearners);
  const annualPrice = sanitizeNonNegative(annualPriceUsd);
  const annualValueUsd = computeLayerCeiling(learners, annualPrice);
  return {
    learners,
    annualPriceUsd: annualPrice,
    annualValueUsd,
    equation: formatLayerEquation(learners, annualPrice, annualValueUsd),
  };
}

function sanitizeNonNegative(n: number): number {
  if (!Number.isFinite(n) || n < 0) return 0;
  return n;
}

function formatCount(n: number): string {
  if (!Number.isFinite(n)) return "0";
  if (Number.isInteger(n)) return n.toLocaleString("en-US");
  return n.toLocaleString("en-US", { maximumFractionDigits: 2 });
}

function unique<T>(items: T[]): T[] {
  return [...new Set(items)];
}
