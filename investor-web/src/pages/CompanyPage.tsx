import { DiligencePage, DiligenceSection } from "@/components/DiligencePage";
import { CaveatNote } from "@/components/CaveatNote";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { TextLink } from "@/components/TextLink";
import { founderProof, sources, teamPlan } from "@/data";
import {
  COMPANY_UPDATED,
  contactProcess,
  ebackpackProofPoints,
  founderRiskMitigation,
  hiringPrinciples,
  openRolesNote,
} from "@/data/companyDeep";

const nav = [
  { id: "timeline", label: "Founder timeline" },
  { id: "ebackpack", label: "eBackpack and integration proof" },
  { id: "team", label: "Current team and 18-month roles" },
  { id: "hiring", label: "Hiring principles" },
  { id: "founder-risk", label: "Founder-risk mitigation" },
  { id: "contact", label: "Contact and references process" },
  { id: "sources", label: "Sources" },
];

const pageSources = sources.filter((s) =>
  ["founder-attested", "funding-plan-internal", "product-state-internal"].includes(s.id),
);

/**
 * Founder story, team plan, hiring principles, references process.
 */
export function CompanyPage() {
  const current = teamPlan.filter((r) => r.presence === "current");
  const planned = teamPlan.filter((r) => r.presence === "planned");

  return (
    <DiligencePage
      eyebrow="Company"
      title="Founder, team, and references process"
      summary="Aleks Clark combines lived homeschool experience, five years building an LMS at scale, and staff-level platform engineering. The company is sole-founder today with an explicit two-hire plan and educator ownership as a risk mitigant."
      updated={COMPANY_UPDATED}
      nav={nav}
      sources={pageSources}
      caveat="Childhood and family photos, children's work, and third-party references require consent and are not published here."
      secondaryAction={{ label: "Primary founder section", href: "/#founder" }}
    >
      <DiligenceSection id="timeline" title="Founder timeline" eyebrow="Proof points">
        <ol className="timeline-list">
          {founderProof.map((event) => (
            <li key={event.id} className="timeline-list__item">
              <RuledCard
                footer={
                  <div className="loop-diagram__footer">
                    <SystemLabel>{event.period}</SystemLabel>
                    <SystemLabel>{event.proofType.replace(/_/g, " ")}</SystemLabel>
                  </div>
                }
              >
                <SystemLabel tone="accent">{event.title}</SystemLabel>
                <p className="type-small" style={{ margin: 0 }}>
                  {event.summary}
                </p>
                {event.metrics && event.metrics.length > 0 ? (
                  <ul className="section-list section-list--compact">
                    {event.metrics.map((m) => (
                      <li key={m.label} className="type-small text-muted">
                        {m.label}: {m.value}
                      </li>
                    ))}
                  </ul>
                ) : null}
              </RuledCard>
            </li>
          ))}
        </ol>
      </DiligenceSection>

      <DiligenceSection
        id="ebackpack"
        title="eBackpack and integration execution"
        eyebrow="LMS proof"
      >
        <RuledGrid columns={2}>
          {ebackpackProofPoints.map((point) => (
            <RuledCard key={point.id}>
              <SystemLabel tone="accent">{point.label}</SystemLabel>
              <p className="type-small" style={{ margin: 0 }}>
                {point.value}
              </p>
            </RuledCard>
          ))}
        </RuledGrid>
        <CaveatNote label="Diligence">
          Scale metrics and employment details are founder-attested and should be confirmed with
          references before external reliance.
        </CaveatNote>
      </DiligenceSection>

      <DiligenceSection
        id="team"
        title="Current team and 18-month roles"
        eyebrow="Compact plan"
      >
        <RuledGrid columns={2}>
          <div>
            <SystemLabel tone="accent">Current</SystemLabel>
            <div className="ruled-stack">
              {current.map((role) => (
                <RuledCard
                  key={role.id}
                  footer={<SystemLabel>{role.timing}</SystemLabel>}
                >
                  <p className="type-small" style={{ margin: 0, fontWeight: 500 }}>
                    {role.title}
                  </p>
                  <ul className="section-list section-list--compact">
                    {role.ownership.map((item) => (
                      <li key={item} className="type-small text-muted">
                        {item}
                      </li>
                    ))}
                  </ul>
                </RuledCard>
              ))}
            </div>
          </div>
          <div>
            <SystemLabel tone="accent">18-month plan</SystemLabel>
            <div className="ruled-stack">
              {planned.map((role) => (
                <RuledCard
                  key={role.id}
                  footer={<SystemLabel>{role.timing}</SystemLabel>}
                >
                  <p className="type-small" style={{ margin: 0, fontWeight: 500 }}>
                    {role.title}
                  </p>
                  <ul className="section-list section-list--compact">
                    {role.ownership.map((item) => (
                      <li key={item} className="type-small text-muted">
                        {item}
                      </li>
                    ))}
                  </ul>
                </RuledCard>
              ))}
            </div>
          </div>
        </RuledGrid>
        <div style={{ marginTop: "var(--primer-space-5)" }}>
          <RuledCard footer={<StateBadge status={openRolesNote.status} />}>
            <SystemLabel tone="accent">Open roles and advisors</SystemLabel>
            <p className="type-small" style={{ margin: 0 }}>
              {openRolesNote.body}
            </p>
          </RuledCard>
        </div>
      </DiligenceSection>

      <DiligenceSection id="hiring" title="Hiring principles" eyebrow="How we hire">
        <RuledGrid columns={2}>
          {hiringPrinciples.map((p) => (
            <RuledCard key={p.id}>
              <SystemLabel tone="accent">{p.title}</SystemLabel>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {p.body}
              </p>
            </RuledCard>
          ))}
        </RuledGrid>
      </DiligenceSection>

      <DiligenceSection
        id="founder-risk"
        title="Founder-risk mitigation"
        eyebrow="Educator ownership and product evidence"
      >
        <ul className="section-list">
          {founderRiskMitigation.map((line) => (
            <li key={line} className="type-small">
              {line}
            </li>
          ))}
        </ul>
      </DiligenceSection>

      <DiligenceSection
        id="contact"
        title="Contact and references process"
        eyebrow="How diligence proceeds"
      >
        <ol className="ladder-list">
          {contactProcess.map((step) => (
            <li key={step.id} className="ladder-list__item">
              <RuledCard
                footer={
                  <SystemLabel>Step {String(step.order).padStart(2, "0")}</SystemLabel>
                }
              >
                <SystemLabel tone="accent">{step.title}</SystemLabel>
                <p className="type-small" style={{ margin: 0 }}>
                  {step.body}
                </p>
              </RuledCard>
            </li>
          ))}
        </ol>
        <div className="diligence-page__actions" style={{ marginTop: "var(--primer-space-5)" }}>
          <TextLink href="/#contact">Discuss the round</TextLink>
          <TextLink href="mailto:aleks@primer.local">Email founder</TextLink>
        </div>
      </DiligenceSection>
    </DiligencePage>
  );
}
