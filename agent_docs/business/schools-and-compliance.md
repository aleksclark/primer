# Schools, interoperability, and compliance

## Can Primer be used in private schools?

Yes. Primer is an LMS, and it can enter private schools early as a supplementary environment for tutoring, intervention, enrichment, projects, supervised study, or remote learners. It does not need to replace the school's primary LMS to deliver value.

A private-school pilot is feasible before full district readiness if it is bounded, supervised, and supported with manual or lightweight rostering. Production use still requires privacy, child-safety, accessibility, identity, administration, and data-governance work.

## Does Primer currently have SIS, SSO, CIPA, and related capabilities?

The existing project documentation describes an LMS API, admin SPA, standards, mastery, assessments, and instruction logs. It does not document production-ready school rostering, institutional SSO, Clever/ClassLink, OneRoster, LTI, school-level consent, district privacy agreements, SOC 2, VPAT/accessibility conformance, or CIPA-supporting controls.

Founder Aleks Clark spent five years as a senior developer at eBackpack, an LMS serving more than half a million students at approximately 60,000 requests per minute and 99.9% uptime. He directly built SIS integrations, OneDrive for Business integration, and assessment capabilities; integrated Skyward, PowerSchool, and Google Classroom for grades and rosters; implemented OAuth 1 and 2 with Google and Microsoft; tagged assessments to Common Core; and supported Common Cartridge content. His broader career includes OpenAPI-documented platforms, distributed systems, cloud infrastructure, and stringent government-customer delivery. SIS, SSO, and LMS integration are familiar engineering territory, not a new category of work.

Therefore the honest pitch answer is:

> Primer is already being built as an LMS, and the founder has deep LMS and integration experience. The specific institutional connectors, certifications, and compliance artefacts are a planned product track and should not be claimed until implemented.

## What private schools require

Private schools vary widely. Independent schools often use Blackbaud or Veracross; religious schools may use FACTS and network-specific systems. A first pilot can use CSV rosters and Google or Microsoft login, but a repeatable product needs:

- normalized organization, school, section, staff, learner, and guardian records;
- Google Workspace and Microsoft Entra ID sign-in;
- generic SAML 2.0 or OIDC for other identity providers;
- roster import and deprovisioning;
- role-based access for school leaders, teachers, aides, parents, and students;
- administrator review of tutor transcripts and flags;
- organization-wide curriculum and AI policy controls;
- export, correction, retention, and deletion workflows;
- accessible learner and administrator interfaces;
- a DPA, security documentation, and subprocessor disclosure;
- a path to coexist with the school's LMS and gradebook.

## Legal obligations versus buyer expectations

### COPPA

COPPA applies directly to the operator when a service is directed to children under 13 or knowingly collects their personal information. Public versus private school does not remove this obligation. The amended COPPA Rule's compliance date was April 22, 2026.

Primer needs:

- appropriate parental consent for direct family use;
- a valid school-authorization flow where schools act for parents under FTC guidance;
- a written information-security program;
- a published retention policy and deletion schedule;
- written assurances from subprocessors;
- clear notices about data categories, recipients, and purposes;
- special treatment for voice recordings and other biometric identifiers;
- no ads or unrelated commercial use of student information.

### FERPA

FERPA applies to educational agencies and institutions receiving U.S. Department of Education funds. Most private K–12 schools are not covered, though exceptions exist. Public districts are covered and will require Primer to operate as a school official under the school's direct control, using records only for the contracted educational purpose.

Private schools still commonly expect FERPA-like practices. Build the public-school data controls early rather than maintaining two standards.

### State student-privacy laws

SOPIPA-style operator laws can apply to vendors serving public or private K–12 schools. Public districts also face state-specific contracting requirements such as California Education Code §49073.1 and New York Education Law §2-d. The practical national baseline is the Student Data Privacy Consortium's National Data Privacy Agreement.

Product commitment: do not train foundation or downstream models on identifiable student data. List model providers as subprocessors and use no-training/limited-retention terms.

### CIPA

CIPA is not a certification a software vendor obtains. It is a condition placed on schools and libraries receiving certain E-rate discounts, including eligible private schools. Primer should support the school's compliance by:

- avoiding unfiltered open-web access;
- filtering unsafe AI inputs and outputs;
- providing adult visibility into student activity;
- generating alerts and exportable logs;
- publishing domains and network requirements for filtering products;
- supporting school-configurable access policies.

Do not say “CIPA compliant” without precise scope. Say that the product is designed not to circumvent school filtering and provides controls that support the school's CIPA obligations.

### Accessibility

Public-school digital content supplied through contracts is subject to ADA Title II. DOJ's WCAG 2.1 AA deadlines are April 26, 2027 for larger entities and April 26, 2028 for smaller entities after the 2026 extension. Private schools may have obligations under ADA Title III and Section 504, with specific exemptions and funding-dependent rules.

Target WCAG 2.2 AA, produce a current VPAT/Accessibility Conformance Report, and test keyboard navigation, screen readers, math accessibility, captions/transcripts, streaming chat announcements, color contrast, and non-timed interaction.

### Data policy gaps to resolve

The current founder hypothesis is to retain raw sessions for one month, delete media three months after inactivity/deletion, and retain LMS records indefinitely unless deletion is requested. Immediate deletion requests would preserve only billing/accounting records and usage statistics required for billing.

Indefinite LMS retention should not become the default product policy. It conflicts with data minimization, COPPA's prohibition on unnecessary retention, school contract deletion requirements, and the parent-ownership principle. Before launch, define purpose-limited retention by record class, explicit family export, prompt account deletion, any legally required billing retention, and a separate consented research policy. No student data is used for model training by default. Any future discounted beta using deidentified data requires legal review, explicit informed consent, and a path to withdraw.

### Security and procurement

COPPA and state laws require reasonable safeguards. Institutional buyers will additionally expect:

- SOC 2 Type II report;
- annual penetration testing and remediation summary;
- encryption in transit and at rest;
- MFA for privileged staff;
- least-privilege RBAC;
- immutable audit logs;
- incident response and breach-notification process;
- U.S. data hosting/processing where contracted;
- public subprocessor and retention registers;
- CoSN K–12CVAT responses;
- cyber liability insurance;
- deletion certification at contract end.

SOC 2 Type II requires an observation period, so preparation must start well before district sales.

## SIS and roster interoperability roadmap

### Private-school pilot

- standalone supplementary Primer LMS or LTI launch from the incumbent LMS;
- CSV import/export;
- Clever SSO first, followed by Google and Microsoft SSO;
- simple section and guardian management;
- optional direct adapters for the first design partner's SIS.

### Repeatable private-school product

- Blackbaud, Veracross, and FACTS integration or middleware that normalizes them;
- generic SAML/OIDC;
- automated nightly sync and deprovisioning;
- gradebook/export workflow.

### District-ready product

- OneRoster 1.2 REST and CSV, with 1.1 compatibility if required;
- Clever and ClassLink rostering plus SSO;
- LTI 1.3 / LTI Advantage for LMS launch, roles, deep linking, and grade passback;
- Ed-Fi only for states/districts that require richer state data exchange;
- 1EdTech certification for implemented standards.

The internal data model should remain independent of any one SIS so the family product and school adapters share the same learner evidence core.

## Administration required for multiple students

- tenant hierarchy: organization → school → grade/section → learner;
- delegated administration and least-privilege roles;
- school-wide and section-level curriculum policy;
- per-feature AI controls and model allowlists;
- transcript review and safety escalation;
- learner transfer without record duplication;
- guardian linking and consent status;
- audit trail for access, exports, changes, and mastery overrides;
- data retention by tenant and record type;
- aggregate reporting without exposing unrelated learners;
- human review for high-stakes mastery or placement decisions.

## Can Primer be used in public schools?

Yes. Families can use Primer supplementarily without a district contract, creating an immediate public-school-adjacent market. Schools can also pilot Primer earlier as a supplementary LMS for a bounded programme. Broad adoption still requires the institutional controls described here.

Primer can coexist with and integrate into the district's incumbent LMS, SIS, identity provider, and gradebook. It may eventually replace some LMS workflows, but requiring rip-and-replace at entry would create unnecessary procurement and migration burden.

A realistic sequence is:

1. direct supplementary remediation used by one family in the school community;
2. supervised intervention, enrichment, project, or tutoring pilot;
3. LTI launch and Clever SSO for one subject, grade, or programme;
4. evidence of implementation fidelity and learning outcomes;
5. OneRoster, grade synchronization, and district trust artefacts;
6. school or network expansion;
7. primary-LMS consideration only after widespread adoption has made Primer the de facto instructional environment.

## School-readiness gates

Do not begin broad district selling until these gates are met:

- repeatable outcomes in the intended grade and subject;
- stable multi-tenant administration and support;
- COPPA program and state-law review;
- signable NDPA/DPA package;
- WCAG conformance and VPAT;
- SOC 2 Type II or a credible dated path accepted by pilot buyers;
- SSO, roster sync, deprovisioning, and audit logs;
- documented AI safety and human-review policy;
- no-training commitment for identifiable student data;
- external assessment of model scoring reliability;
- implementation model a school can execute without founder presence.
