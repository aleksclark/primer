import type { MarketLayer } from "./types";

/**
 * Vertical expansion layers. Every record sets additive: false.
 * Overlapping populations share overlapGroup keys and must never be summed.
 *
 * Arithmetic (reproducible from source fields):
 *   3_400_000 * 600 = 2_040_000_000
 *   14_610_000 * 1_200 = 17_532_000_000
 *   631_672 * 3_600 = 2_274_019_200
 *   1_159_670 * 3_600 = 4_174_812_000
 */
export const marketLayers: MarketLayer[] = [
  {
    id: "base-homeschool-nheri-mid",
    package: "base",
    segment: "U.S. homeschool learners (modeled mid)",
    learners: 3_400_000,
    annualRevenuePerLearner: 600,
    modeledCeiling: 2_040_000_000,
    sourceYear: "2024–25",
    observedOrModeled: "modeled",
    overlapGroup: "homeschool-us",
    additive: false,
    caveat:
      "NHERI modeled mid estimate. Overlapping expansion layer — do not sum with Core/Premier. Not SAM, forecast, or current revenue.",
    sourceIds: ["nheri-2024-25", "nces-pfi-2023"],
    displayLabel: "BASE · 3.4M modeled homeschool learners × $600 = $2.04B",
  },
  {
    id: "core-public-grades-5-8",
    package: "core",
    segment: "U.S. public grades 5–8 learners (family-paid population ceiling)",
    learners: 14_610_000,
    annualRevenuePerLearner: 1_200,
    modeledCeiling: 17_532_000_000,
    sourceYear: "fall 2024",
    observedOrModeled: "observed",
    overlapGroup: "public-k12-us",
    additive: false,
    caveat:
      "Theoretical full-population ceiling for parent-paid Core supplementation. Not a serviceable market. Overlaps other family layers — do not sum.",
    sourceIds: ["nces-digest-203-10-2025"],
    displayLabel: "CORE · 14.61M public grades 5–8 learners × $1,200 = $17.53B",
  },
  {
    id: "premier-nais",
    package: "premier",
    segment: "NAIS DASL reporting universe",
    learners: 631_672,
    annualRevenuePerLearner: 3_600,
    modeledCeiling: 2_274_019_200,
    sourceYear: "2024–25",
    observedOrModeled: "observed",
    overlapGroup: "premium-private-us",
    additive: false,
    caveat:
      "NAIS reporting schools only. Overlaps the $15K+ private tuition cohort — alternative boundary, not additive.",
    sourceIds: ["nais-facts-2024-25"],
    displayLabel: "PREMIER · 631,672 NAIS learners × $3,600 = $2.27B",
  },
  {
    id: "premier-private-15k-plus",
    package: "premier",
    segment: "Private-school learners in $15K+ tuition schools",
    learners: 1_159_670,
    annualRevenuePerLearner: 3_600,
    modeledCeiling: 4_174_812_000,
    sourceYear: "2020–21",
    observedOrModeled: "observed",
    overlapGroup: "premium-private-us",
    additive: false,
    caveat:
      "NCES 2020–21 sticker-price tuition bands; vintage caveat required. Overlaps NAIS cohort — do not sum.",
    sourceIds: ["nces-digest-205-50"],
    displayLabel: "PREMIER PRIVATE · 1.16M learners in $15K+ tuition schools × $3,600 = $4.17B",
  },
  {
    id: "premier-nonsectarian-15k-plus",
    package: "premier",
    segment: "Nonsectarian private learners in $15K+ tuition schools",
    learners: 600_680,
    annualRevenuePerLearner: 3_600,
    modeledCeiling: 2_162_448_000,
    sourceYear: "2020–21",
    observedOrModeled: "observed",
    overlapGroup: "premium-private-us",
    additive: false,
    caveat:
      "Subset of the $15K+ private cohort. Alternative premium boundary — do not add to NAIS or all-private $15K+.",
    sourceIds: ["nces-digest-205-50"],
    displayLabel: "PREMIER NONSECTARIAN · 600,680 learners × $3,600 = $2.16B",
  },
  {
    id: "institutional-public-intervention-example",
    package: "institutional",
    segment: "Illustrative public intervention cohort (not a national TAM)",
    learners: 500_000,
    annualRevenuePerLearner: 1_200,
    modeledCeiling: 600_000_000,
    sourceYear: "model",
    observedOrModeled: "modeled",
    overlapGroup: "public-intervention-illustrative",
    additive: false,
    caveat:
      "Illustrative only: school-paid deployments target roughly 5–15% of enrollment as instructional capacity, not software seats. Not a national SAM.",
    sourceIds: ["nssa-budget-guidance", "nces-digest-203-10-2025"],
    displayLabel: "INSTITUTIONAL · illustrative targeted intervention at $1,200/learner",
  },
];
