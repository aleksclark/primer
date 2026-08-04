import type { StatusLabel } from "./types";

/** School expansion, interoperability, and compliance roadmap. */

export interface RoadmapGate {
  id: string;
  order: number;
  name: string;
  detail: string;
  status: Extract<StatusLabel, "LIVE" | "IN_DEVELOPMENT" | "PLANNED">;
}

export interface ComplianceItem {
  id: string;
  name: string;
  posture: string;
  status: StatusLabel;
  notes: string;
}

export interface BuyingMotion {
  id: string;
  market: "public" | "private" | "family-adjacent";
  name: string;
  sequence: string[];
  status: StatusLabel;
}

export const SCHOOLS_UPDATED = "2026-08-04";

export const schoolReadinessGates: RoadmapGate[] = [
  {
    id: "family-privacy-coppa",
    order: 1,
    name: "Family privacy and COPPA programme",
    detail:
      "Parental consent, notices, retention/deletion schedule, subprocessor assurances, and written information-security programme before broad child accounts.",
    status: "IN_DEVELOPMENT",
  },
  {
    id: "lti-clever",
    order: 2,
    name: "LTI launch and Clever SSO",
    detail:
      "Supplementary school entry via LTI 1.3 launch and Clever SSO after family retention evidence. CSV rostering acceptable for first pilots.",
    status: "PLANNED",
  },
  {
    id: "org-roles-transcript",
    order: 3,
    name: "Organizational roles and transcript review",
    detail:
      "Tenant hierarchy, delegated admin, teacher/aide roles, and administrator review of tutor transcripts and safety flags.",
    status: "PLANNED",
  },
  {
    id: "oneroster-grades",
    order: 4,
    name: "OneRoster and grade synchronization",
    detail:
      "OneRoster 1.2 REST/CSV with 1.1 compatibility if required; grade passback and deprovisioning for repeatable school product.",
    status: "PLANNED",
  },
  {
    id: "wcag-vpat",
    order: 5,
    name: "WCAG conformance and VPAT",
    detail:
      "Target WCAG 2.2 AA with keyboard, screen reader, math accessibility, captions/transcripts, and a current VPAT/ACR.",
    status: "PLANNED",
  },
  {
    id: "dpa-ndpa",
    order: 6,
    name: "DPA / NDPA readiness",
    detail:
      "Signable national data privacy agreement package, subprocessor register, and school-official / direct-control posture for FERPA-covered buyers.",
    status: "PLANNED",
  },
  {
    id: "soc2",
    order: 7,
    name: "SOC 2 Type II",
    detail:
      "Observation-period audit. Preparation starts well before district sales; pilots may accept a dated path with compensating controls.",
    status: "PLANNED",
  },
  {
    id: "state-contracting",
    order: 8,
    name: "State-specific contracting",
    detail:
      "SOPIPA-style operator laws, CA Ed Code §49073.1, NY Ed Law §2-d, and other state packs as buyers require.",
    status: "PLANNED",
  },
  {
    id: "sis-identity",
    order: 9,
    name: "Broader SIS and identity adapters",
    detail:
      "Blackbaud, Veracross, FACTS, ClassLink, generic SAML/OIDC, Google Workspace and Microsoft Entra ID beyond Clever-first entry.",
    status: "PLANNED",
  },
  {
    id: "primary-lms",
    order: 10,
    name: "Primary-LMS migration path",
    detail:
      "Consider primary LMS only after widespread de facto adoption. Entry never requires rip-and-replace of the incumbent LMS.",
    status: "PLANNED",
  },
];

export const complianceItems: ComplianceItem[] = [
  {
    id: "coppa",
    name: "COPPA",
    posture:
      "Direct obligation when serving children under 13. Parental consent for family use; school-authorization flow where schools act for parents under FTC guidance. Amended rule compliance date April 22, 2026.",
    status: "IN_DEVELOPMENT",
    notes: "No ads or unrelated commercial use of student information. Voice/biometric identifiers need special treatment.",
  },
  {
    id: "ferpa",
    name: "FERPA",
    posture:
      "Applies to funded educational agencies. Most private K–12 schools are not covered, but public districts will require school-official / direct-control handling. Build one high standard rather than two.",
    status: "PLANNED",
    notes: "Private schools still commonly expect FERPA-like practices.",
  },
  {
    id: "cipa",
    name: "CIPA support",
    posture:
      "Not a vendor certification. Designed not to circumvent school filtering: no unfiltered open web, unsafe AI I/O filtering, adult visibility, exportable logs, publishable network requirements.",
    status: "PLANNED",
    notes: "Never claim “CIPA compliant” without precise scope.",
  },
  {
    id: "accessibility",
    name: "Accessibility (WCAG / VPAT)",
    posture:
      "Target WCAG 2.2 AA. Public-school digital content faces ADA Title II deadlines (2027/2028 after 2026 extension). Private schools may have Title III / §504 exposure.",
    status: "PLANNED",
    notes: "Keyboard, screen readers, math accessibility, captions/transcripts, contrast, non-timed interaction.",
  },
  {
    id: "dpa",
    name: "DPA / NDPA",
    posture: "Signable SDPC National Data Privacy Agreement package and school DPAs before district expansion.",
    status: "PLANNED",
    notes: "No training foundation or downstream models on identifiable student data by default.",
  },
  {
    id: "soc2-item",
    name: "SOC 2 Type II",
    posture: "Required observation period. Pair with pen test summary, encryption, MFA for privileged staff, RBAC, audit logs, incident response, and cyber liability insurance.",
    status: "PLANNED",
    notes: "Start preparation well before broad district selling.",
  },
  {
    id: "retention",
    name: "Retention and deletion",
    posture:
      "Indefinite LMS retention must not be the default. Purpose-limited retention by record class, family export, prompt deletion, legally required billing retention only. Separate consented research policy if ever used.",
    status: "IN_DEVELOPMENT",
    notes: "Current founder hypothesis (raw sessions ~1 month, media ~3 months, LMS indefinite) is a gap to resolve before launch.",
  },
];

export const integrationStrategy = [
  {
    id: "private-pilot",
    name: "Private-school pilot",
    items: [
      "Standalone supplementary LMS or LTI launch from incumbent LMS",
      "CSV import/export",
      "Clever SSO first, then Google and Microsoft SSO",
      "Simple section and guardian management",
      "Optional direct adapter for the first design partner's SIS",
    ],
  },
  {
    id: "repeatable-private",
    name: "Repeatable private-school product",
    items: [
      "Blackbaud, Veracross, and FACTS integration or normalizing middleware",
      "Generic SAML / OIDC",
      "Automated nightly sync and deprovisioning",
      "Gradebook / export workflow",
      "School-authored instructional guardrails and branded experiences for elite partners",
    ],
  },
  {
    id: "district-ready",
    name: "District-ready product",
    items: [
      "OneRoster 1.2 REST and CSV (1.1 compatibility if required)",
      "Clever and ClassLink rostering plus SSO",
      "LTI 1.3 / LTI Advantage: launch, roles, deep linking, grade passback",
      "Ed-Fi only where states/districts require richer exchange",
      "1EdTech certification for implemented standards",
    ],
  },
];

export const buyingMotions: BuyingMotion[] = [
  {
    id: "family-to-school",
    market: "family-adjacent",
    name: "Family-to-school supplementary motion",
    sequence: [
      "Direct family subscription alongside public or private school",
      "Teacher or counselor notices improvement / reduced parent burden",
      "Bounded one-student or small-group school pilot",
      "LTI + Clever supplementary deployment",
      "Roster/grade integration after proof",
    ],
    status: "HYPOTHESIS",
  },
  {
    id: "public-intervention-motion",
    market: "public",
    name: "Targeted public intervention",
    sequence: [
      "Family-adjacent proof in the community",
      "Supervised intervention, enrichment, project, or tutoring pilot",
      "LTI launch and Clever SSO for one subject, grade, or programme",
      "Implementation fidelity and learning outcomes evidence",
      "OneRoster, grade sync, and district trust artefacts",
      "School or network expansion",
      "Primary-LMS consideration only after de facto adoption",
    ],
    status: "PLANNED",
  },
  {
    id: "elite-private-motion",
    market: "private",
    name: "Elite independent and boarding support",
    sequence: [
      "Parent-paid community benefit or learning-support pilot",
      "Study-hall / dorm academic support wedge (boarding)",
      "School-authored pedagogy guardrails and teacher-visible transcripts",
      "Blackbaud / Veracross path and branded experiences",
      "Tuition-embedded programme only after staffing-substitute value is clear",
    ],
    status: "PLANNED",
  },
];

export const schoolHonestPitch =
  "Primer is already being built as an LMS, and the founder has deep LMS and integration experience. The specific institutional connectors, certifications, and compliance artefacts are a planned product track and should not be claimed until implemented.";
