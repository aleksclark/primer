import { DiligencePage, DiligenceSection } from "@/components/DiligencePage";
import { CaveatNote } from "@/components/CaveatNote";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { adoptionLadder, sources } from "@/data";
import {
  buyingMotions,
  complianceItems,
  integrationStrategy,
  SCHOOLS_UPDATED,
  schoolHonestPitch,
  schoolReadinessGates,
} from "@/data/schoolsRoadmap";

const nav = [
  { id: "honest-pitch", label: "Honest school posture" },
  { id: "family-to-school", label: "Family-to-school motion" },
  { id: "buying-motions", label: "Public vs private buying motions" },
  { id: "readiness-gates", label: "School-readiness gates" },
  { id: "integrations", label: "LTI / Clever / OneRoster / SIS" },
  { id: "blackbaud-veracross", label: "Blackbaud / Veracross strategy" },
  { id: "compliance", label: "Privacy, COPPA, FERPA, CIPA, accessibility, DPA, SOC 2" },
  { id: "primary-lms", label: "Primary-LMS path" },
  { id: "sources", label: "Sources" },
];

const pageSources = sources.filter((s) =>
  [
    "founder-attested",
    "product-state-internal",
    "seed-readiness-internal",
    "nais-facts-2024-25",
    "nssa-budget-guidance",
  ].includes(s.id),
);

/**
 * Schools expansion, interoperability, and compliance diligence.
 */
export function SchoolsPage() {
  return (
    <DiligencePage
      eyebrow="Schools"
      title="Supplementary entry to primary LMS path"
      summary="Family-to-school motion, targeted public intervention, elite independent and boarding support, interoperability roadmap, and compliance gates. Institutional connectors are planned — not claimed as live."
      updated={SCHOOLS_UPDATED}
      nav={nav}
      sources={pageSources}
      caveat={schoolHonestPitch}
      secondaryAction={{ label: "Go-to-market ladder", href: "/#go-to-market" }}
    >
      <DiligenceSection id="honest-pitch" title="Honest school posture" eyebrow="Public claim">
        <RuledCard attention>
          <p className="type-body prose-measure" style={{ margin: 0 }}>
            {schoolHonestPitch}
          </p>
        </RuledCard>
      </DiligenceSection>

      <DiligenceSection
        id="family-to-school"
        title="Family-to-school supplementary motion"
        eyebrow="Adoption ladder"
      >
        <ol className="ladder-list">
          {adoptionLadder.map((rung) => (
            <li key={rung.id} className="ladder-list__item">
              <RuledCard
                footer={
                  <div className="loop-diagram__footer">
                    <SystemLabel>Rung {String(rung.order).padStart(2, "0")}</SystemLabel>
                    <StateBadge status={rung.status} />
                  </div>
                }
              >
                <SystemLabel tone="accent">{rung.name}</SystemLabel>
                <dl className="detail-list">
                  <div>
                    <dt>
                      <SystemLabel>Buyer</SystemLabel>
                    </dt>
                    <dd className="type-small">{rung.buyer}</dd>
                  </div>
                  <div>
                    <dt>
                      <SystemLabel>Proof</SystemLabel>
                    </dt>
                    <dd className="type-small">{rung.proof}</dd>
                  </div>
                  <div>
                    <dt>
                      <SystemLabel>Integration</SystemLabel>
                    </dt>
                    <dd className="type-small">{rung.integration}</dd>
                  </div>
                </dl>
              </RuledCard>
            </li>
          ))}
        </ol>
      </DiligenceSection>

      <DiligenceSection
        id="buying-motions"
        title="Public versus private buying motions"
        eyebrow="Sequences"
      >
        <RuledGrid columns={1}>
          {buyingMotions.map((motion) => (
            <RuledCard
              key={motion.id}
              footer={
                <div className="loop-diagram__footer">
                  <SystemLabel>{motion.market}</SystemLabel>
                  <StateBadge status={motion.status} />
                </div>
              }
            >
              <SystemLabel tone="accent">{motion.name}</SystemLabel>
              <ol className="section-list">
                {motion.sequence.map((step) => (
                  <li key={step} className="type-small">
                    {step}
                  </li>
                ))}
              </ol>
            </RuledCard>
          ))}
        </RuledGrid>
      </DiligenceSection>

      <DiligenceSection
        id="readiness-gates"
        title="School-readiness gates"
        eyebrow="Do not skip"
      >
        <CaveatNote label="Gate">
          Do not begin broad district selling until outcomes, multi-tenant admin, COPPA/state review,
          NDPA/DPA, WCAG/VPAT, SOC 2 path, SSO/roster/deprovisioning, AI safety policy, no-training
          commitment, scoring reliability, and implementable support are in place.
        </CaveatNote>
        <ol className="ladder-list" style={{ marginTop: "var(--primer-space-5)" }}>
          {schoolReadinessGates.map((gate) => (
            <li key={gate.id} className="ladder-list__item">
              <RuledCard
                footer={
                  <div className="loop-diagram__footer">
                    <SystemLabel>Gate {String(gate.order).padStart(2, "0")}</SystemLabel>
                    <StateBadge status={gate.status} />
                  </div>
                }
              >
                <p className="type-small" style={{ margin: 0, fontWeight: 500 }}>
                  {gate.name}
                </p>
                <p className="type-small text-muted" style={{ margin: 0 }}>
                  {gate.detail}
                </p>
              </RuledCard>
            </li>
          ))}
        </ol>
      </DiligenceSection>

      <DiligenceSection
        id="integrations"
        title="LTI, Clever, OneRoster, and SIS roadmap"
        eyebrow="Interoperability"
      >
        <RuledGrid columns={1}>
          {integrationStrategy.map((block) => (
            <RuledCard key={block.id} footer={<SystemLabel>{block.name}</SystemLabel>}>
              <SystemLabel tone="accent">{block.name}</SystemLabel>
              <ul className="section-list">
                {block.items.map((item) => (
                  <li key={item} className="type-small">
                    {item}
                  </li>
                ))}
              </ul>
            </RuledCard>
          ))}
        </RuledGrid>
      </DiligenceSection>

      <DiligenceSection
        id="blackbaud-veracross"
        title="Blackbaud and Veracross strategy"
        eyebrow="Elite private"
      >
        <RuledGrid columns={2}>
          <RuledCard footer={<StateBadge status="PLANNED" />}>
            <SystemLabel tone="accent">Independent schools</SystemLabel>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              Many independent schools run Blackbaud or Veracross. First pilots can use CSV rosters
              and Google/Microsoft login. Repeatable product needs normalized org/section/staff/learner
              records and automated sync — via direct adapters or middleware that normalizes both.
            </p>
          </RuledCard>
          <RuledCard footer={<StateBadge status="PLANNED" />}>
            <SystemLabel tone="accent">Boarding wedge</SystemLabel>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              Evening study hall, dorm academic support, EAL scaffolding, asynchronous help across
              time zones, and guardian reporting — staffing substitute without a specialist for every
              subject and dorm. Small reference market with high strategic value.
            </p>
          </RuledCard>
        </RuledGrid>
        <p className="type-small text-muted prose-measure" style={{ margin: 0 }}>
          Religious schools may use FACTS and network-specific systems; treat them as additional
          adapters on the same internal learner-evidence model, not a separate product fork.
        </p>
      </DiligenceSection>

      <DiligenceSection
        id="compliance"
        title="Privacy, COPPA, FERPA, CIPA support, accessibility, DPA, SOC 2"
        eyebrow="Compliance roadmap"
      >
        <RuledGrid columns={2}>
          {complianceItems.map((item) => (
            <RuledCard
              key={item.id}
              footer={<StateBadge status={item.status} />}
            >
              <SystemLabel tone="accent">{item.name}</SystemLabel>
              <p className="type-small" style={{ margin: 0 }}>
                {item.posture}
              </p>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {item.notes}
              </p>
            </RuledCard>
          ))}
        </RuledGrid>
      </DiligenceSection>

      <DiligenceSection
        id="primary-lms"
        title="Primary-LMS path after de facto adoption"
        eyebrow="Earn, do not demand"
      >
        <RuledCard>
          <p className="type-small prose-measure" style={{ margin: 0 }}>
            Primer can coexist with and integrate into the district or school incumbent LMS, SIS,
            identity provider, and gradebook. Requiring rip-and-replace at entry creates unnecessary
            procurement and migration burden. Primary-LMS consideration comes only after widespread
            adoption has made Primer the de facto instructional environment — and after readiness
            gates above are met.
          </p>
        </RuledCard>
      </DiligenceSection>
    </DiligencePage>
  );
}
