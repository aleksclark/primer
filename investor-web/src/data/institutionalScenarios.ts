import type { ObservationKind, StatusLabel } from "./types";

/**
 * Institutional contracting scenarios — separate from family expansion layers.
 * Do not combine with Base/Core/Premier family ceilings.
 */
export interface InstitutionalScenario {
  id: string;
  name: string;
  buyer: string;
  description: string;
  defaultLearners: number;
  defaultAnnualPriceUsd: number;
  minLearners: number;
  maxLearners: number;
  minAnnualPriceUsd: number;
  maxAnnualPriceUsd: number;
  priceStepUsd: number;
  learnerStep: number;
  unitLabel: string;
  sourceYear: string;
  observedOrModeled: ObservationKind;
  additive: false;
  overlapGroup: string;
  caveat: string;
  status: StatusLabel;
  sourceIds: string[];
}

export const institutionalScenarios: InstitutionalScenario[] = [
  {
    id: "public-intervention",
    name: "Public intervention",
    buyer: "Public school / district",
    description:
      "Targeted tutoring or intervention for a defined cohort, funded as instructional capacity rather than whole-school software seats.",
    defaultLearners: 500_000,
    defaultAnnualPriceUsd: 1_200,
    minLearners: 1_000,
    maxLearners: 2_000_000,
    minAnnualPriceUsd: 600,
    maxAnnualPriceUsd: 2_500,
    priceStepUsd: 50,
    learnerStep: 1_000,
    unitLabel: "targeted learners",
    sourceYear: "model",
    observedOrModeled: "modeled",
    additive: false,
    overlapGroup: "public-intervention-illustrative",
    caveat:
      "Illustrative only. School-paid deployments typically target roughly 5–15% of enrollment. Not a national SAM and not additive with family Core.",
    status: "HYPOTHESIS",
    sourceIds: ["nssa-budget-guidance", "nces-digest-203-10-2025"],
  },
  {
    id: "elite-learning-support",
    name: "Elite learning support",
    buyer: "Elite independent / boarding school",
    description:
      "School-bought learning support, study-hall capacity, or parent-paid community access priced against tutoring and specialist support.",
    defaultLearners: 50_000,
    defaultAnnualPriceUsd: 3_600,
    minLearners: 100,
    maxLearners: 631_672,
    minAnnualPriceUsd: 1_000,
    maxAnnualPriceUsd: 5_000,
    priceStepUsd: 100,
    learnerStep: 100,
    unitLabel: "supported learners",
    sourceYear: "model",
    observedOrModeled: "modeled",
    additive: false,
    overlapGroup: "elite-learning-support-illustrative",
    caveat:
      "Alternative premium boundary to family Premier. Count a learner once under the party paying Primer.",
    status: "HYPOTHESIS",
    sourceIds: ["nais-facts-2024-25", "seed-readiness-internal"],
  },
  {
    id: "microschool-academic-core",
    name: "Microschool academic core",
    buyer: "Microschool / hybrid founder",
    description:
      "Primer as the academic operating system for a lean microschool or hybrid programme.",
    defaultLearners: 5_000,
    defaultAnnualPriceUsd: 2_500,
    minLearners: 20,
    maxLearners: 50_000,
    minAnnualPriceUsd: 1_500,
    maxAnnualPriceUsd: 6_500,
    priceStepUsd: 100,
    learnerStep: 20,
    unitLabel: "enrolled learners",
    sourceYear: "model",
    observedOrModeled: "modeled",
    additive: false,
    overlapGroup: "microschool-illustrative",
    caveat:
      "National microschool site counts are unreliable. Size from reachable associations and ESA-provider lists, not headline site estimates.",
    status: "HYPOTHESIS",
    sourceIds: ["seed-readiness-internal"],
  },
  {
    id: "international-school-license",
    name: "International school license",
    buyer: "International school / group",
    description:
      "School- or group-level annual license across enrolled learners, typically lower ARPU and broader seat coverage.",
    defaultLearners: 100_000,
    defaultAnnualPriceUsd: 150,
    minLearners: 500,
    maxLearners: 7_600_000,
    minAnnualPriceUsd: 50,
    maxAnnualPriceUsd: 300,
    priceStepUsd: 10,
    learnerStep: 500,
    unitLabel: "enrolled learners",
    sourceYear: "model",
    observedOrModeled: "modeled",
    additive: false,
    overlapGroup: "international-school-illustrative",
    caveat:
      "Requires IB/IGCSE/A-level/AP readiness, multilingual support, and regional compliance. Not additive with U.S. family layers.",
    status: "HYPOTHESIS",
    sourceIds: ["seed-readiness-internal"],
  },
];
