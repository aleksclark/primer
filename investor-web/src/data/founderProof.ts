import type { FounderProofEvent } from "./types";

/**
 * Resume-backed and lived-experience timeline.
 * Childhood/family photos require explicit consent — not encoded here.
 */
export const founderProof: FounderProofEvent[] = [
  {
    id: "homeschooled-student",
    period: "Grades K–7, 9, and 10",
    title: "Homeschooled student in Peru",
    summary:
      "Aleks Clark was homeschooled in grades K–7, 9, and 10 while living in Peru with limited U.S.-accredited options. A typical day was about three hours, with flexibility to work ahead. He benefited from self-direction, reading, and learning by doing.",
    proofType: "lived_experience",
    sourceIds: ["founder-attested"],
    status: "LIVE",
  },
  {
    id: "homeschool-parent-abeka",
    period: "Three years through grades 5–8",
    title: "Homeschool parent using Abeka",
    summary:
      "Used Abeka across all subjects to homeschool two daughters for three years through grades 5–8. Found it rigorous but inflexible, easy to game in assessment, and responsible for more than ten parent hours per week.",
    proofType: "lived_experience",
    metrics: [{ label: "Parent hours per week", value: ">10" }],
    sourceIds: ["founder-attested"],
    status: "LIVE",
  },
  {
    id: "ebackpack-lms",
    period: "Five years",
    title: "Built eBackpack LMS infrastructure",
    summary:
      "Spent five years building eBackpack, a learning management system focused on ease of use. Direct work on SIS integrations, assessment, standards tagging, and content interchange.",
    proofType: "execution",
    metrics: [
      { label: "Students served", value: ">500,000" },
      { label: "Peak request rate", value: "~60,000 req/min" },
      { label: "Uptime", value: "99.9%" },
    ],
    sourceIds: ["founder-attested"],
    status: "LIVE",
  },
  {
    id: "ebackpack-integrations",
    period: "eBackpack tenure",
    title: "School interoperability experience",
    summary:
      "Integrated Skyward, PowerSchool, and Google Classroom for rosters and grades; implemented OAuth 1/2 with Google and Microsoft; tagged assessments to Common Core; supported Common Cartridge; built OneDrive for Business integration.",
    proofType: "platform",
    metrics: [
      { label: "Integrations", value: "Skyward · PowerSchool · Google Classroom · OneDrive · OAuth · Common Core · Common Cartridge" },
    ],
    sourceIds: ["founder-attested"],
    status: "LIVE",
  },
  {
    id: "staff-engineering",
    period: "Career to date",
    title: "Staff-level platform and AI infrastructure",
    summary:
      "Staff-level software engineer with experience in AI agent infrastructure, distributed systems, cloud platforms, reliability, and technical leadership across commercial and government customers.",
    proofType: "platform",
    sourceIds: ["founder-attested"],
    status: "LIVE",
  },
  {
    id: "primer-active-use",
    period: "Current",
    title: "Building and using Primer with his sixth-grade son",
    summary:
      "Has begun using command-teacher and PrimerTV with his sixth-grade son while broader subject tutoring, projects, and assessment are built. Early family use is discovery, not outcome evidence.",
    proofType: "active_use",
    metrics: [
      { label: "Founder hours invested", value: "~40" },
      { label: "Cash spent", value: "~$1,000" },
    ],
    sourceIds: ["founder-attested", "product-state-internal"],
    status: "LIVE",
  },
  {
    id: "sole-founder",
    period: "Current team",
    title: "Sole team member today",
    summary:
      "Aleks is currently the sole team member and owns product, curriculum, safety, privacy, and accessibility. The 18-month plan adds one or two high-leverage employees: an experienced educator/product owner and an experienced product/platform builder.",
    proofType: "execution",
    sourceIds: ["founder-attested", "funding-plan-internal"],
    status: "LIVE",
  },
];
