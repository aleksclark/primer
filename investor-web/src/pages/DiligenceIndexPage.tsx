import { DiligencePage, DiligenceSection } from "@/components/DiligencePage";
import { SeedScorecardExplorer } from "@/components/explorers";
import { CaveatNote } from "@/components/CaveatNote";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { TextLink } from "@/components/TextLink";
import { fundingPlan, productState, sources } from "@/data";
import {
  DILIGENCE_UPDATED,
  diligenceIndex,
  materialRisks,
  publicPrivateBoundary,
} from "@/data/materialRisks";
import { formatUsd } from "@/lib/format";
import { schoolReadinessGates } from "@/data/schoolsRoadmap";

const nav = [
  { id: "index", label: "Public diligence index" },
  { id: "product-state-summary", label: "Product state summary" },
  { id: "funding", label: "Funding plan summary" },
  { id: "seed-economics", label: "Seed-readiness economics" },
  { id: "risks", label: "Material risks" },
  { id: "compliance-roadmap", label: "Compliance roadmap" },
  { id: "boundaries", label: "Public vs gated boundary" },
  { id: "data-room", label: "Gated data-room placeholder" },
  { id: "sources", label: "Sources" },
];

const pageSources = sources.filter((s) =>
  [
    "funding-plan-internal",
    "seed-readiness-internal",
    "product-state-internal",
    "founder-attested",
  ].includes(s.id),
);

/**
 * Public diligence index — data-room style, no private secrets.
 */
export function DiligenceIndexPage() {
  const live = productState.filter((p) => p.status === "LIVE").length;
  const building = productState.filter((p) => p.status === "IN_DEVELOPMENT").length;
  const planned = productState.filter((p) => p.status === "PLANNED").length;

  return (
    <DiligencePage
      eyebrow="Diligence"
      title="Public diligence index"
      summary="Structured record of product state, market model, funding plan, seed economics, material risks, compliance roadmap, and sources. Optional gated data room is a placeholder only — no private contracts, family data, security internals, or cap table on this site."
      updated={DILIGENCE_UPDATED}
      nav={nav}
      sources={pageSources}
      printable
      caveat="Public pages answer core underwriting questions. Gated materials require a direct request and appropriate confidentiality."
      secondaryAction={{ label: "Discuss the round", href: "/#contact" }}
    >
      <DiligenceSection id="index" title="Public index" eyebrow="Navigate">
        <div className="diligence-index-list">
          {diligenceIndex.map((entry) => (
            <article key={entry.id} className="diligence-index-list__item">
              <div className="diligence-index-list__meta">
                <SystemLabel
                  tone={entry.access === "public" ? "accent" : "attention"}
                >
                  {entry.access === "public" ? "Public" : "Gated placeholder"}
                </SystemLabel>
                <SystemLabel>Updated {entry.updated}</SystemLabel>
              </div>
              <h3 className="type-h3" style={{ margin: 0 }}>
                <TextLink href={entry.href}>{entry.title}</TextLink>
              </h3>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {entry.summary}
              </p>
            </article>
          ))}
        </div>
      </DiligenceSection>

      <DiligenceSection
        id="product-state-summary"
        title="Product state summary"
        eyebrow="Inventory counts"
      >
        <RuledGrid columns={3}>
          <RuledCard footer={<SystemLabel>Demonstrable</SystemLabel>}>
            <SystemLabel tone="accent">LIVE</SystemLabel>
            <p className="type-h2" style={{ margin: 0 }}>
              {live}
            </p>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              command-teacher, PrimerTV, LMS core, Ultralogical, and related live rows
            </p>
          </RuledCard>
          <RuledCard footer={<SystemLabel>Active build</SystemLabel>}>
            <SystemLabel>IN DEVELOPMENT</SystemLabel>
            <p className="type-h2" style={{ margin: 0 }}>
              {building}
            </p>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              Subject tutors, planning, assessment, mastery records, family admin
            </p>
          </RuledCard>
          <RuledCard footer={<SystemLabel>Roadmap</SystemLabel>}>
            <SystemLabel>PLANNED</SystemLabel>
            <p className="type-h2" style={{ margin: 0 }}>
              {planned}
            </p>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              Habit guards, projects, LTI/Clever, mobile
            </p>
          </RuledCard>
        </RuledGrid>
        <p className="type-small" style={{ margin: "var(--primer-space-4) 0 0" }}>
          <TextLink href="/demo">Open full product demo and inventory</TextLink>
        </p>
      </DiligenceSection>

      <DiligenceSection id="funding" title="Funding plan summary" eyebrow="Working target">
        <RuledGrid columns={2}>
          <RuledCard footer={<StateBadge status={fundingPlan.status} />}>
            <SystemLabel tone="accent">{fundingPlan.roundName}</SystemLabel>
            <p className="type-h2" style={{ margin: 0 }}>
              {formatUsd(fundingPlan.amountUsd, { compact: true })}
            </p>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              {fundingPlan.runwayMonths} months · team of {fundingPlan.teamSize} ·{" "}
              {fundingPlan.instrument}
            </p>
            <p className="type-small" style={{ margin: 0 }}>
              People {formatUsd(fundingPlan.peopleCost18MoUsd, { compact: true })} · Non-payroll{" "}
              {formatUsd(fundingPlan.nonPayrollBaseUsd, { compact: true })} · Contingency{" "}
              {formatUsd(fundingPlan.contingencyUsd, { compact: true })}
            </p>
          </RuledCard>
          <RuledCard footer={<SystemLabel>Milestones (excerpt)</SystemLabel>}>
            <ul className="section-list section-list--compact">
              {fundingPlan.milestones.slice(0, 6).map((m) => (
                <li key={m} className="type-small">
                  {m}
                </li>
              ))}
            </ul>
            <TextLink href="/#round">Full use-of-funds on primary page</TextLink>
          </RuledCard>
        </RuledGrid>
        <CaveatNote label="Not public">
          Cap table, option grants, exact financing terms, and total dilution are not published on
          this site.
        </CaveatNote>
      </DiligenceSection>

      <DiligenceSection
        id="seed-economics"
        title="Seed-readiness economics"
        eyebrow="Scorecard"
      >
        <SeedScorecardExplorer />
      </DiligenceSection>

      <DiligenceSection id="risks" title="Material risks" eyebrow="Thirteen items">
        <ol className="ladder-list">
          {materialRisks.map((risk) => (
            <li key={risk.id} className="ladder-list__item">
              <RuledCard
                footer={
                  <div className="loop-diagram__footer">
                    <SystemLabel>Risk {String(risk.order).padStart(2, "0")}</SystemLabel>
                    <StateBadge status={risk.status} />
                  </div>
                }
              >
                <SystemLabel tone="attention">{risk.title}</SystemLabel>
                <p className="type-small" style={{ margin: 0 }}>
                  {risk.summary}
                </p>
                <p className="type-small text-muted" style={{ margin: 0 }}>
                  <strong className="type-label">Mitigation · </strong>
                  {risk.mitigation}
                </p>
              </RuledCard>
            </li>
          ))}
        </ol>
      </DiligenceSection>

      <DiligenceSection
        id="compliance-roadmap"
        title="Compliance roadmap"
        eyebrow="School gates excerpt"
      >
        <div className="funds-table-wrap">
          <table className="funds-table">
            <caption className="sr-only">School readiness compliance gates</caption>
            <thead>
              <tr>
                <th scope="col">#</th>
                <th scope="col">Gate</th>
                <th scope="col">Detail</th>
                <th scope="col">Status</th>
              </tr>
            </thead>
            <tbody>
              {schoolReadinessGates.map((gate) => (
                <tr key={gate.id}>
                  <td className="type-small">{String(gate.order).padStart(2, "0")}</td>
                  <th scope="row" className="type-small">
                    {gate.name}
                  </th>
                  <td className="type-small text-muted">{gate.detail}</td>
                  <td>
                    <StateBadge status={gate.status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="type-small" style={{ margin: "var(--primer-space-4) 0 0" }}>
          <TextLink href="/schools">Full schools and compliance page</TextLink>
        </p>
      </DiligenceSection>

      <DiligenceSection
        id="boundaries"
        title="Public versus gated boundary"
        eyebrow="Enforced"
      >
        <ul className="section-list">
          {publicPrivateBoundary.map((line) => (
            <li key={line} className="type-small">
              {line}
            </li>
          ))}
        </ul>
      </DiligenceSection>

      <DiligenceSection
        id="data-room"
        title="Gated data-room placeholder"
        eyebrow="Not a portal"
      >
        <RuledCard attention footer={<SystemLabel tone="attention">Placeholder only</SystemLabel>}>
          <SystemLabel tone="attention">No private materials hosted here</SystemLabel>
          <p className="type-small prose-measure" style={{ margin: 0 }}>
            A future NDA-gated room may hold deeper financial model detail, diligence-appropriate
            architecture, reference introductions, and employment verification. This public site
            intentionally excludes private contracts, family or student data, detailed security
            architecture, live credentials, and cap-table documents.
          </p>
          <p className="type-small" style={{ margin: 0 }}>
            Request access via <TextLink href="/#contact">Discuss the round</TextLink> or{" "}
            <TextLink href="mailto:aleks@primer.local">aleks@primer.local</TextLink>.
          </p>
        </RuledCard>
      </DiligenceSection>
    </DiligencePage>
  );
}
