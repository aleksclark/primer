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

export {
  buildTargetTranscript,
  DEMO_UPDATED,
  liveSurfaces,
  SYNTHETIC_LEARNER,
  targetExperienceSteps,
} from "./demoScript";
export {
  bloomCaveat,
  claimBoundaries,
  EVIDENCE_UPDATED,
  evidenceStages,
  learningThresholds,
  nullAdversePolicy,
} from "./evidencePlan";
export {
  COMPANY_UPDATED,
  contactProcess,
  ebackpackProofPoints,
  founderRiskMitigation,
  hiringPrinciples,
  openRolesNote,
} from "./companyDeep";
export {
  contractingModels,
  MARKET_UPDATED,
  opportunityMap,
  overlapMethodology,
  publicInterventionRationale,
  tutoringPriceComparisons,
} from "./marketDeep";
export {
  DILIGENCE_UPDATED,
  diligenceIndex,
  materialRisks,
  publicPrivateBoundary,
} from "./materialRisks";
export {
  buyingMotions,
  complianceItems,
  integrationStrategy,
  SCHOOLS_UPDATED,
  schoolHonestPitch,
  schoolReadinessGates,
} from "./schoolsRoadmap";

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
