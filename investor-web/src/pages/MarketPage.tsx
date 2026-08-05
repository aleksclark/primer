import { DiligencePage, DiligenceSection } from "@/components/DiligencePage";
import {
  InstitutionalExplorer,
  MarketExplorer,
  PackageLadderExplorer,
} from "@/components/explorers";
import { CaveatNote } from "@/components/CaveatNote";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { SourceLinkList } from "@/components/SourceLink";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { marketLayers, pricingTiers, sources } from "@/data";
import {
  contractingModels,
  MARKET_UPDATED,
  opportunityMap,
  overlapMethodology,
  publicInterventionRationale,
  tutoringPriceComparisons,
} from "@/data/marketDeep";
import { formatCeiling, formatLearners, formatUsd } from "@/lib/format";

const nav = [
  { id: "source-year-tables", label: "Source-year market tables" },
  { id: "expansion-layers", label: "Base / Core / Premier layers" },
  { id: "package-ladder", label: "Package ladder" },
  { id: "contracting", label: "Family vs institutional contracting" },
  { id: "premium-private", label: "Premium private cohorts" },
  { id: "price-comparisons", label: "Tutoring price comparisons" },
  { id: "public-intervention", label: "Public intervention rationale" },
  { id: "opportunity-map", label: "International / microschool / special support" },
  { id: "overlap", label: "Overlap methodology" },
  { id: "institutional-explorer", label: "Institutional scenarios" },
  { id: "sources", label: "Sources" },
];

function resolveSources(ids: string[]) {
  return sources.filter((s) => ids.includes(s.id));
}

const allSourceIds = [
  ...new Set([
    ...marketLayers.flatMap((l) => l.sourceIds),
    ...tutoringPriceComparisons.flatMap((t) => t.sourceIds),
    "nssa-budget-guidance",
    "edchoice-tutoring-2025",
    "nais-facts-2024-25",
    "nces-digest-205-50",
    "nces-digest-203-10-2025",
    "nheri-2024-25",
    "seed-readiness-internal",
  ]),
];

/**
 * Diligence-depth market model with printable assumption summary.
 */
export function MarketPage() {
  const premiumLayers = marketLayers.filter((l) => l.overlapGroup === "premium-private-us");

  return (
    <DiligencePage
      eyebrow="Market model"
      title="Assumptions, expansion layers, and contracting models"
      summary="Source-year tables, non-summing Base/Core/Premier ceilings, family versus institutional contracting, premium private cohorts, tutoring price comparisons, and overlap methodology. Every layer carries additive: false."
      updated={MARKET_UPDATED}
      nav={nav}
      sources={resolveSources(allSourceIds)}
      printable
      caveat="Modeled ceilings are not forecasts, SAM, or current revenue. Do not sum overlapping populations."
      secondaryAction={{ label: "Primary market section", href: "/#market" }}
    >
      <DiligenceSection
        id="source-year-tables"
        title="Source-year market tables"
        eyebrow="Reproducible equations"
      >
        <div className="funds-table-wrap">
          <table className="funds-table">
            <caption className="sr-only">
              Market layers with source year, learners, ARPU, and modeled ceiling
            </caption>
            <thead>
              <tr>
                <th scope="col">Layer</th>
                <th scope="col">Source year</th>
                <th scope="col">Kind</th>
                <th scope="col">Learners</th>
                <th scope="col">Annual ARPU</th>
                <th scope="col">Modeled ceiling</th>
                <th scope="col">Overlap group</th>
              </tr>
            </thead>
            <tbody>
              {marketLayers.map((layer) => (
                <tr key={layer.id}>
                  <th scope="row">
                    <div className="type-small" style={{ fontWeight: 500 }}>
                      {layer.package.toUpperCase()}
                    </div>
                    <div className="type-small text-muted">{layer.segment}</div>
                  </th>
                  <td className="type-small">{layer.sourceYear}</td>
                  <td className="type-small">{layer.observedOrModeled}</td>
                  <td className="type-small">{formatLearners(layer.learners)}</td>
                  <td className="type-small">{formatUsd(layer.annualRevenuePerLearner)}</td>
                  <td className="type-small">{formatCeiling(layer.modeledCeiling)}</td>
                  <td className="type-small text-muted">{layer.overlapGroup}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <CaveatNote label="Do not sum">
          additive is false on every row. Overlapping groups share learners and must not be combined
          into one TAM.
        </CaveatNote>
      </DiligenceSection>

      <DiligenceSection
        id="expansion-layers"
        title="Base / Core / Premier expansion layers"
        eyebrow="Interactive"
      >
        <MarketExplorer />
      </DiligenceSection>

      <DiligenceSection id="package-ladder" title="Package ladder" eyebrow="Working prices">
        <PackageLadderExplorer />
        <div className="funds-table-wrap" style={{ marginTop: "var(--primer-space-5)" }}>
          <table className="funds-table">
            <caption className="sr-only">Pricing tiers summary</caption>
            <thead>
              <tr>
                <th scope="col">Tier</th>
                <th scope="col">Monthly</th>
                <th scope="col">Annual</th>
                <th scope="col">Status</th>
                <th scope="col">Competitive frame</th>
              </tr>
            </thead>
            <tbody>
              {pricingTiers.map((tier) => (
                <tr key={tier.id}>
                  <th scope="row" className="type-small">
                    {tier.name}
                  </th>
                  <td className="type-small">{formatUsd(tier.monthlyPriceUsd)}</td>
                  <td className="type-small">{formatUsd(tier.annualRevenuePerLearnerUsd)}</td>
                  <td>
                    <StateBadge status={tier.status} />
                  </td>
                  <td className="type-small text-muted">{tier.competitiveFrame}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </DiligenceSection>

      <DiligenceSection
        id="contracting"
        title="Family versus institutional contracting"
        eyebrow="Who pays"
      >
        <RuledGrid columns={2}>
          {contractingModels.map((model) => (
            <RuledCard
              key={model.id}
              footer={<StateBadge status={model.status} />}
            >
              <SystemLabel tone="accent">{model.name}</SystemLabel>
              <p className="type-small" style={{ margin: 0 }}>
                <strong className="type-label">Payer · </strong>
                {model.payer}
              </p>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {model.howItWorks}
              </p>
              <p className="type-small" style={{ margin: 0 }}>
                {model.priceFrame}
              </p>
            </RuledCard>
          ))}
        </RuledGrid>
      </DiligenceSection>

      <DiligenceSection
        id="premium-private"
        title="Premium private-school cohorts"
        eyebrow="Alternative boundaries"
      >
        <RuledGrid columns={3}>
          {premiumLayers.map((layer) => (
            <RuledCard
              key={layer.id}
              footer={
                <SystemLabel>
                  {layer.sourceYear} · {layer.observedOrModeled}
                </SystemLabel>
              }
            >
              <SystemLabel tone="accent">{layer.package.toUpperCase()}</SystemLabel>
              <p className="type-h3" style={{ margin: 0 }}>
                {formatCeiling(layer.modeledCeiling)}
              </p>
              <p className="type-small" style={{ margin: 0 }}>
                {layer.segment}
              </p>
              <p className="type-label text-muted" style={{ margin: 0 }}>
                {formatLearners(layer.learners)} × {formatUsd(layer.annualRevenuePerLearner)}
              </p>
              <CaveatNote>{layer.caveat}</CaveatNote>
              <SourceLinkList sources={resolveSources(layer.sourceIds)} />
            </RuledCard>
          ))}
        </RuledGrid>
        <p className="type-small text-muted prose-measure" style={{ margin: 0 }}>
          Elite schools buy via parent-paid community benefit, targeted learning-support programmes,
          or tuition-embedded capacity — not ordinary technology budgets (~$524/learner NAIS median
          tech expense).
        </p>
      </DiligenceSection>

      <DiligenceSection
        id="price-comparisons"
        title="Tutoring price comparisons"
        eyebrow="Value frame"
      >
        <div className="funds-table-wrap">
          <table className="funds-table">
            <caption className="sr-only">Tutoring and Primer price comparisons</caption>
            <thead>
              <tr>
                <th scope="col">Offer</th>
                <th scope="col">Price</th>
                <th scope="col">Unit</th>
                <th scope="col">Note</th>
              </tr>
            </thead>
            <tbody>
              {tutoringPriceComparisons.map((row) => (
                <tr key={row.id}>
                  <th scope="row" className="type-small">
                    {row.offer}
                  </th>
                  <td className="type-small">{row.price}</td>
                  <td className="type-small text-muted">{row.unit}</td>
                  <td className="type-small text-muted">{row.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </DiligenceSection>

      <DiligenceSection
        id="public-intervention"
        title="Public intervention pricing rationale"
        eyebrow="Instructional capacity"
      >
        <ul className="section-list">
          {publicInterventionRationale.map((line) => (
            <li key={line} className="type-small">
              {line}
            </li>
          ))}
        </ul>
      </DiligenceSection>

      <DiligenceSection
        id="opportunity-map"
        title="International, microschool, and special-support map"
        eyebrow="Non-overlapping opportunities"
      >
        <div className="funds-table-wrap">
          <table className="funds-table">
            <caption className="sr-only">
              Opportunity map with buyer, ARPU, unit, and strategic role
            </caption>
            <thead>
              <tr>
                <th scope="col">Opportunity</th>
                <th scope="col">Buyer</th>
                <th scope="col">Working annual ARPU</th>
                <th scope="col">Addressable unit</th>
                <th scope="col">Strategic role</th>
              </tr>
            </thead>
            <tbody>
              {opportunityMap.map((row) => (
                <tr key={row.id}>
                  <th scope="row" className="type-small">
                    {row.opportunity}
                  </th>
                  <td className="type-small">{row.buyer}</td>
                  <td className="type-small">{row.annualArpu}</td>
                  <td className="type-small text-muted">{row.addressableUnit}</td>
                  <td className="type-small text-muted">{row.strategicRole}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <CaveatNote label="Do not sum">
          Rows are alternative contracts. Count a learner once under the party paying Primer.
          additive remains false on every opportunity.
        </CaveatNote>
      </DiligenceSection>

      <DiligenceSection id="overlap" title="Overlap and deduplication methodology" eyebrow="Rules">
        <ol className="section-list">
          {overlapMethodology.map((rule) => (
            <li key={rule} className="type-small">
              {rule}
            </li>
          ))}
        </ol>
      </DiligenceSection>

      <DiligenceSection
        id="institutional-explorer"
        title="Institutional contracting scenarios"
        eyebrow="Illustrative"
      >
        <InstitutionalExplorer />
      </DiligenceSection>
    </DiligencePage>
  );
}
