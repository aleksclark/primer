import type { StatusLabel } from "./types";

/** Diligence-depth market tables beyond the primary explorer. */

export interface TutoringPriceComparison {
  id: string;
  offer: string;
  price: string;
  unit: string;
  note: string;
  sourceIds: string[];
}

export interface OpportunityMapRow {
  id: string;
  opportunity: string;
  buyer: string;
  annualArpu: string;
  addressableUnit: string;
  strategicRole: string;
  additive: false;
}

export interface ContractingModel {
  id: string;
  name: string;
  payer: string;
  howItWorks: string;
  priceFrame: string;
  status: StatusLabel;
}

export const MARKET_UPDATED = "2026-08-04";

export const tutoringPriceComparisons: TutoringPriceComparison[] = [
  {
    id: "marketplace-tutoring",
    offer: "Marketplace human tutoring (Care.com / Wyzant range)",
    price: "$25–$60",
    unit: "per hour",
    note: "Ordinary subject tutoring; does not include longitudinal curriculum ownership.",
    sourceIds: ["wyzant-rates", "edchoice-tutoring-2025"],
  },
  {
    id: "managed-membership",
    offer: "Managed tutoring membership (e.g. Varsity-style bundles)",
    price: "~$299",
    unit: "per month for ~4 hours + classes",
    note: "Human hours remain the scarce unit; Primer prices persistent coverage instead.",
    sourceIds: ["edchoice-tutoring-2025"],
  },
  {
    id: "test-prep",
    offer: "Premium test-prep tutoring",
    price: "~$175+",
    unit: "per hour",
    note: "High hourly rates for narrow exam outcomes.",
    sourceIds: ["edchoice-tutoring-2025"],
  },
  {
    id: "parent-wtp",
    offer: "Stated parent willingness to pay for tutoring",
    price: "~$357",
    unit: "per month (stated average)",
    note: "EdChoice/Morning Consult stated WTP — not observed Primer conversion.",
    sourceIds: ["edchoice-tutoring-2025"],
  },
  {
    id: "nssa-hit",
    offer: "High-impact tutoring budget guidance (districts)",
    price: "$1,200–$2,500",
    unit: "per learner per year",
    note: "Stanford NSSA range for instructional capacity, not commodity software seats.",
    sourceIds: ["nssa-budget-guidance"],
  },
  {
    id: "primer-base",
    offer: "Primer Base (working price)",
    price: "$50 / $600",
    unit: "per month / per year per learner",
    note: "Hypothesis. Seed economics measured on Base. Less than one hour of many human tutors.",
    sourceIds: ["seed-readiness-internal"],
  },
  {
    id: "primer-core",
    offer: "Primer Core (working price)",
    price: "$100 / $1,200",
    unit: "per month / per year per learner",
    note: "Hypothesis. Full core academics; sits at low end of HIT budgets.",
    sourceIds: ["seed-readiness-internal", "nssa-budget-guidance"],
  },
  {
    id: "primer-premier",
    offer: "Primer Premier (working price)",
    price: "$300 / $3,600",
    unit: "per month / per year per learner",
    note: "Hypothesis. Persistent multi-domain coverage vs stacked human specialists.",
    sourceIds: ["seed-readiness-internal", "edchoice-tutoring-2025"],
  },
];

export const contractingModels: ContractingModel[] = [
  {
    id: "family-direct",
    name: "Family direct subscription",
    payer: "Parent / guardian (card or ESA where eligible)",
    howItWorks:
      "Household buys Base, Core, or Premier for one or more learners. Parent owns curriculum direction and privacy consent.",
    priceFrame: "$50 / $100 / $300 per learner per month",
    status: "HYPOTHESIS",
  },
  {
    id: "parent-paid-school-community",
    name: "Parent-paid school community benefit",
    payer: "Parent, with school endorsement or light integration",
    howItWorks:
      "School recommends or integrates Primer; families still subscribe. Low procurement burden for the school.",
    priceFrame: "Family ladder prices; school may sponsor subsets",
    status: "HYPOTHESIS",
  },
  {
    id: "targeted-intervention",
    name: "School-paid targeted intervention",
    payer: "School / district programme budget",
    howItWorks:
      "School buys Primer for a defined cohort (typically 5–15% of enrollment) as tutoring or learning-support capacity with dosage and outcomes reporting.",
    priceFrame: "~$1,200 per supported learner per year",
    status: "HYPOTHESIS",
  },
  {
    id: "tuition-embedded",
    name: "Tuition-embedded academic programme",
    payer: "School (priced against tuition and staffing, not IT budget)",
    howItWorks:
      "Primer becomes part of the instructional model for learning support, study hall, or broader academic capacity after supplementary proof.",
    priceFrame: "Programme-level; Premier-class for elite support",
    status: "PLANNED",
  },
];

export const opportunityMap: OpportunityMapRow[] = [
  {
    id: "base-family",
    opportunity: "Base family tutor",
    buyer: "Parent",
    annualArpu: "$600",
    addressableUnit: "Focused homeschool / supplement learner",
    strategicRole: "Initial acquisition and product proof",
    additive: false,
  },
  {
    id: "core-family",
    opportunity: "Core family education",
    buyer: "Parent / ESA",
    annualArpu: "$1,200",
    addressableUnit: "Full-spectrum homeschool / supplement learner",
    strategicRole: "ARPU expansion and curriculum breadth",
    additive: false,
  },
  {
    id: "premier-family",
    opportunity: "Premier family education",
    buyer: "Parent / ESA",
    annualArpu: "$3,600",
    addressableUnit: "High-intent, private-school, specialist-support learner",
    strategicRole: "Premium margin and long LTV",
    additive: false,
  },
  {
    id: "public-intervention",
    opportunity: "Targeted public/private intervention",
    buyer: "School",
    annualArpu: "~$1,200",
    addressableUnit: "Supported learner, not whole enrollment",
    strategicRole: "Institutional outcomes and scale",
    additive: false,
  },
  {
    id: "elite-support",
    opportunity: "Elite learning support",
    buyer: "School / family",
    annualArpu: "$1,000–$3,600+",
    addressableUnit: "Supported learner",
    strategicRole: "Premium references and staffing substitution",
    additive: false,
  },
  {
    id: "microschool",
    opportunity: "Microschool academic core",
    buyer: "School founder",
    annualArpu: "$1,500–$6,500",
    addressableUnit: "Enrolled learner",
    strategicRole: "Highest institutional ownership of workflow",
    additive: false,
  },
  {
    id: "international",
    opportunity: "International school platform",
    buyer: "School / group",
    annualArpu: "$50–$300",
    addressableUnit: "All enrolled learners",
    strategicRole: "Large global school-side volume",
    additive: false,
  },
  {
    id: "special-support",
    opportunity: "Specialized learning support",
    buyer: "Family / funded programme",
    annualArpu: "Premier-class ($3,600 working)",
    addressableUnit: "Learner needing accommodations / EF support",
    strategicRole: "High WTP with high evidence and liability bar",
    additive: false,
  },
];

export const overlapMethodology = [
  "Every market layer sets additive: false. UI and copy never auto-sum Base, Core, and Premier ceilings.",
  "Layers that share learners share an overlapGroup key (for example premium-private-us for NAIS and $15K+ private cohorts).",
  "A learner is counted once under the party paying Primer — family subscription or school contract, not both.",
  "ESA and choice funding are distribution channels, not a separate population to add on top of private/homeschool counts without deduplication.",
  "Institutional scenarios are illustrative programme math, not national SAMs, and stay separate from family expansion explorers.",
  "Source year and observed-vs-modeled labels travel with every equation so vintage and modeling choices stay visible.",
];

export const publicInterventionRationale = [
  "Districts will not buy $1,200 software licenses for every student. The offer is targeted instructional capacity for a defined cohort.",
  "Typical school-side targeting is roughly 5–15% of enrollment for intervention, mandated remediation, summer learning, or learning support.",
  "Example equation: 5,000-student district × 10% cohort × $1,200 = $600,000 annual programme — subject to procurement, implementation, and outcomes.",
  "Reporting must include dosage, active use, standards progress, and cost per successful learner. Some programmes may place 10–50% of payment at risk.",
  "Family-paid Core supplementation of public grades 5–8 is a separate population ceiling ($17.53B theoretical) and must not be blended with school-paid intervention.",
];
