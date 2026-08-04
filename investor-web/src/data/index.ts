import { competitorCategories } from "./competitorCategories";
import { founderProof } from "./founderProof";
import { fundingPlan } from "./fundingPlan";
import { marketLayers } from "./marketLayers";
import { pricingTiers } from "./pricingTiers";
import { productState } from "./productState";
import { researchClaims } from "./researchClaims";
import { sections } from "./sections";
import { seedScorecard } from "./seedScorecard";
import { sources } from "./sources";
import type { InvestorDataPackage } from "./types";
import { toDisplayStatus } from "./types";

export * from "./types";
export {
  competitorCategories,
  founderProof,
  fundingPlan,
  marketLayers,
  pricingTiers,
  productState,
  researchClaims,
  sections,
  seedScorecard,
  sources,
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
};

export default investorData;
