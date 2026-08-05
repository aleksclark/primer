/**
 * Unit tests for market arithmetic, formatting, and overlap prevention.
 * Run: npx tsx tests/marketMath.test.ts
 */
import assert from "node:assert/strict";
import { marketLayers } from "../src/data/marketLayers.ts";
import {
  formatCeiling,
  formatLearners,
  formatMetricValue,
  formatUsd,
} from "../src/lib/format.ts";
import {
  computeInstitutionalProgramme,
  computeLayerCeiling,
  defaultsFromLayer,
  formatLayerEquation,
  isDefaultInputs,
  recalculateLayer,
  tryCombineLayerCeilings,
} from "../src/lib/marketMath.ts";

let passed = 0;
let failed = 0;

function test(name: string, fn: () => void): void {
  try {
    fn();
    passed += 1;
    console.log(`  ✓ ${name}`);
  } catch (err) {
    failed += 1;
    console.error(`  ✗ ${name}`);
    console.error(`    ${err instanceof Error ? err.message : String(err)}`);
  }
}

console.log("marketMath + format");

test("computeLayerCeiling multiplies learners by ARPU", () => {
  assert.equal(computeLayerCeiling(3_400_000, 600), 2_040_000_000);
  assert.equal(computeLayerCeiling(14_610_000, 1_200), 17_532_000_000);
  assert.equal(computeLayerCeiling(631_672, 3_600), 2_274_019_200);
  assert.equal(computeLayerCeiling(1_159_670, 3_600), 4_174_812_000);
});

test("computeLayerCeiling clamps invalid inputs to zero product", () => {
  assert.equal(computeLayerCeiling(-10, 600), 0);
  assert.equal(computeLayerCeiling(100, Number.NaN), 0);
  assert.equal(computeLayerCeiling(Number.POSITIVE_INFINITY, 10), 0);
});

test("sourced market layers stay internally consistent", () => {
  for (const layer of marketLayers) {
    assert.equal(
      layer.modeledCeiling,
      computeLayerCeiling(layer.learners, layer.annualRevenuePerLearner),
      layer.id,
    );
    assert.equal(layer.additive, false, layer.id);
  }
});

test("recalculateLayer derives ceiling and marks dirty state", () => {
  const base = marketLayers.find((l) => l.id === "base-homeschool-nheri-mid");
  assert.ok(base);
  const same = recalculateLayer(base, defaultsFromLayer(base));
  assert.equal(same.modeledCeiling, base.modeledCeiling);
  assert.equal(same.dirty, false);
  assert.equal(same.observedOrModeled, base.observedOrModeled);
  assert.equal(same.additive, false);

  const edited = recalculateLayer(base, {
    learners: 1_000_000,
    annualRevenuePerLearner: 600,
  });
  assert.equal(edited.modeledCeiling, 600_000_000);
  assert.equal(edited.dirty, true);
  assert.equal(edited.observedOrModeled, "derived");
  assert.match(edited.equation, /1,000,000 learners/);
});

test("isDefaultInputs detects sourced defaults", () => {
  const layer = marketLayers[0];
  assert.equal(isDefaultInputs(layer, defaultsFromLayer(layer)), true);
  assert.equal(
    isDefaultInputs(layer, {
      learners: layer.learners + 1,
      annualRevenuePerLearner: layer.annualRevenuePerLearner,
    }),
    false,
  );
});

test("tryCombineLayerCeilings always refuses multi-layer totals", () => {
  const family = marketLayers
    .filter((l) => l.package !== "institutional")
    .slice(0, 3)
    .map((l) => recalculateLayer(l, defaultsFromLayer(l)));

  const result = tryCombineLayerCeilings(family);
  assert.equal(result.allowed, false);
  assert.match(result.reason, /do not sum|alternative ceilings|overlapping/i);
  assert.ok(result.layerIds.length === family.length);
});

test("tryCombineLayerCeilings blocks family + institutional blend", () => {
  const base = marketLayers.find((l) => l.id === "base-homeschool-nheri-mid");
  const inst = marketLayers.find((l) => l.id === "institutional-public-intervention-example");
  assert.ok(base && inst);
  const result = tryCombineLayerCeilings([
    recalculateLayer(base, defaultsFromLayer(base)),
    recalculateLayer(inst, defaultsFromLayer(inst)),
  ]);
  assert.equal(result.allowed, false);
  assert.match(result.reason, /institutional|deduplication|family/i);
});

test("overlap group shared across premier alternatives is reported", () => {
  const premier = marketLayers
    .filter((l) => l.overlapGroup === "premium-private-us")
    .map((l) => recalculateLayer(l, defaultsFromLayer(l)));
  const result = tryCombineLayerCeilings(premier);
  assert.equal(result.allowed, false);
  assert.ok(result.overlappingGroups.includes("premium-private-us"));
  assert.match(result.reason, /premium-private-us|do not sum/i);
});

test("formatLayerEquation is copyable and stable", () => {
  const eq = formatLayerEquation(3_400_000, 600);
  assert.equal(eq, "3,400,000 learners × $600/yr = $2,040,000,000");
});

test("institutional programme math", () => {
  const prog = computeInstitutionalProgramme(500_000, 1_200);
  assert.equal(prog.annualValueUsd, 600_000_000);
  assert.match(prog.equation, /500,000/);
});

test("formatLearners and formatCeiling", () => {
  assert.equal(formatLearners(3_400_000), "3.4M");
  assert.equal(formatLearners(14_610_000), "14.61M");
  assert.equal(formatLearners(631_672), "631,672");
  assert.equal(formatCeiling(2_040_000_000), "$2.04B");
  assert.equal(formatCeiling(600_000_000), "$600M");
  assert.equal(formatUsd(1200), "$1,200");
});

test("formatMetricValue never turns NOT_YET_MEASURED into zero", () => {
  assert.equal(formatMetricValue("NOT_YET_MEASURED", "percent"), "NOT YET MEASURED");
  assert.equal(formatMetricValue("NOT_YET_MEASURED", "usd_per_year"), "NOT YET MEASURED");
  assert.equal(formatMetricValue(0, "percent"), "0%");
  assert.notEqual(formatMetricValue("NOT_YET_MEASURED", "learners"), "0");
});

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
