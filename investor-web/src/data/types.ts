/**
 * Shared contracts for investor-pitch structured content.
 * Status labels match content-spec System C vocabulary.
 */

/** Product and claim status labels from content-spec.md */
export type StatusLabel =
  | "LIVE"
  | "IN_DEVELOPMENT"
  | "PLANNED"
  | "HYPOTHESIS"
  | "EVIDENCE_NEEDED";

/** Display form used in UI copy (spaces instead of underscores). */
export type StatusLabelDisplay =
  | "LIVE"
  | "IN DEVELOPMENT"
  | "PLANNED"
  | "HYPOTHESIS"
  | "EVIDENCE NEEDED";

export type PackageTier = "base" | "core" | "premier" | "institutional";

export type ObservationKind = "observed" | "modeled" | "derived";

export type SourceQualityTier =
  | "primary_official"
  | "peer_reviewed"
  | "industry_report"
  | "advocacy_modeled"
  | "vendor_official"
  | "founder_attested"
  | "internal_plan";

export type MetricCurrentValue = number | "NOT_YET_MEASURED";

export interface Source {
  id: string;
  title: string;
  organization: string;
  url: string;
  publicationDate: string;
  accessedDate: string;
  qualityTier: SourceQualityTier;
  notes?: string;
}

export interface ProductCapability {
  id: string;
  name: string;
  description: string;
  status: Extract<StatusLabel, "LIVE" | "IN_DEVELOPMENT" | "PLANNED">;
  /** Required when status is LIVE — demonstrable artifact reference. */
  artifactUrl?: string;
  artifactLabel?: string;
  owner?: string;
  nearTermMilestone?: string;
  notes?: string;
}

export interface PricingTier {
  id: PackageTier;
  name: string;
  monthlyPriceUsd: number;
  annualRevenuePerLearnerUsd: number;
  scope: string[];
  primaryBuyers: string[];
  includedUsage: string;
  cogsState: StatusLabel;
  status: StatusLabel;
  competitiveFrame: string;
  notes?: string;
}

/**
 * Market expansion layer. `additive` is always false so UI never
 * auto-sums Base/Core/Premier ceilings into a blended TAM.
 */
export interface MarketLayer {
  id: string;
  package: PackageTier;
  segment: string;
  learners: number;
  annualRevenuePerLearner: number;
  modeledCeiling: number;
  sourceYear: string;
  observedOrModeled: ObservationKind;
  /** Layers that share learners must share an overlapGroup. */
  overlapGroup: string;
  /** Always false — overlapping layers must never be summed. */
  additive: false;
  caveat: string;
  sourceIds: string[];
  displayLabel: string;
}

export interface SeedScorecardMetric {
  id: string;
  name: string;
  unit: string;
  floor: number;
  target: number;
  current: MetricCurrentValue;
  definition: string;
  /** Higher is better unless direction is "lower_is_better". */
  direction: "higher_is_better" | "lower_is_better";
  sourceIds: string[];
  caveat: string;
  status: StatusLabel;
}

export interface ResearchClaim {
  id: string;
  effect: string;
  effectSizeSd?: number;
  population: string;
  study: string;
  safeClaim: string;
  caveat: string;
  sourceIds: string[];
  status: StatusLabel;
}

export type CompetitorSupport = "yes" | "partial" | "no" | "unknown";

export interface CompetitorCategory {
  id: string;
  category: string;
  examples: string[];
  whatCustomersBuy: string;
  primerResponse: string;
  capabilities: {
    adultDirected: CompetitorSupport;
    longitudinalPlanning: CompetitorSupport;
    crossSubjectHabits: CompetitorSupport;
    masteryEvidence: CompetitorSupport;
    projectsAsInstruction: CompetitorSupport;
    offScreenWork: CompetitorSupport;
    householdFirst: CompetitorSupport;
  };
  sourceIds: string[];
  notes?: string;
}

export interface FounderProofEvent {
  id: string;
  period: string;
  title: string;
  summary: string;
  proofType: "lived_experience" | "execution" | "platform" | "active_use";
  metrics?: { label: string; value: string }[];
  sourceIds: string[];
  status: StatusLabel;
}

export interface FundingLineItem {
  id: string;
  category: string;
  leanUsd: number;
  fullUsd: number;
  driver: string;
}

export interface FundingPlan {
  roundName: string;
  instrument: string;
  amountUsd: number;
  runwayMonths: number;
  teamSize: number;
  annualCashSalaryUsd: number;
  employmentLoadRate: number;
  contingencyRate: number;
  peopleCost18MoUsd: number;
  nonPayrollBaseUsd: number;
  contingencyUsd: number;
  totalUsd: number;
  compensationAnnotation: string;
  lineItems: FundingLineItem[];
  milestones: string[];
  status: StatusLabel;
  sourceIds: string[];
  publicationBlockers: string[];
}

export interface SectionCta {
  label: string;
  href: string;
}

export interface SectionManifestEntry {
  id: string;
  navLabel: string;
  eyebrow?: string;
  headline: string;
  body: string;
  subhead?: string;
  cta?: SectionCta;
  secondaryCta?: SectionCta;
  status: StatusLabel;
  artifactRequirement: string;
  citationIds: string[];
  inlineCaveat?: string;
  publicationBlocker?: string;
  proofLine?: string;
}

export interface InvestorDataPackage {
  productState: ProductCapability[];
  pricingTiers: PricingTier[];
  marketLayers: MarketLayer[];
  seedScorecard: SeedScorecardMetric[];
  researchClaims: ResearchClaim[];
  competitorCategories: CompetitorCategory[];
  founderProof: FounderProofEvent[];
  fundingPlan: FundingPlan;
  sources: Source[];
  sections: SectionManifestEntry[];
}

export function toDisplayStatus(status: StatusLabel): StatusLabelDisplay {
  switch (status) {
    case "IN_DEVELOPMENT":
      return "IN DEVELOPMENT";
    case "EVIDENCE_NEEDED":
      return "EVIDENCE NEEDED";
    default:
      return status;
  }
}
