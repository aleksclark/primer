import { adoptionLadder } from "./adoptionLadder";
import { competitorCategories } from "./competitorCategories";
import { founderProof } from "./founderProof";
import { fundingPlan } from "./fundingPlan";
import { instructionalLoop } from "./instructionalLoop";
import { institutionalScenarios } from "./institutionalScenarios";
import { marketLayers } from "./marketLayers";
import { beachheadJobs, problemPoints } from "./problemPoints";
import { pricingTiers } from "./pricingTiers";
import { productState } from "./productState";
import { researchClaims } from "./researchClaims";
import { sections } from "./sections";
import { seedScorecard } from "./seedScorecard";
import { sources } from "./sources";
import { teachingExamples } from "./teachingExamples";
import { teamPlan } from "./teamPlan";
import type { InvestorDataPackage } from "./types";
import { toDisplayStatus } from "./types";

export * from "./types";
export {
  adoptionLadder,
  beachheadJobs,
  competitorCategories,
  founderProof,
  fundingPlan,
  instructionalLoop,
  institutionalScenarios,
  marketLayers,
  pricingTiers,
  problemPoints,
  productState,
  researchClaims,
  sections,
  seedScorecard,
  sources,
  teachingExamples,
  teamPlan,
  toDisplayStatus,
};

/** Aggregated package consumed by Phase 1+ presentation components. */
export const investorData: InvestorDataPackage = {
  productState,
  pricingTiers,
  marketLayers,
  seedScorecard,
  researchClaims,
  competitorCategories,
  founderProof,
  fundingPlan,
  sources,
  sections,
  instructionalLoop,
  teachingExamples,
  adoptionLadder,
  problemPoints,
  beachheadJobs,
  teamPlan,
};

export default investorData;
