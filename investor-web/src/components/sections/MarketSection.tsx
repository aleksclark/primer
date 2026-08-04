import { CaveatNote } from "@/components/CaveatNote";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { SourceLinkList } from "@/components/SourceLink";
import { SystemLabel } from "@/components/SystemLabel";
import { marketLayers, sources } from "@/data";

function formatBillions(value: number): string {
  return `$${(value / 1_000_000_000).toFixed(2)}B`;
}

function formatLearners(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n % 1_000_000 === 0 ? 0 : 2)}M`;
  return n.toLocaleString();
}

function resolveSources(ids: string[]) {
  return sources.filter((s) => ids.includes(s.id));
}

/**
 * Four primary non-additive expansion layers with equations and caveats.
 * Never auto-sums ceilings into a blended TAM.
 */
export function MarketSection() {
  // Primary narrative layers only — exclude illustrative institutional and subset rows.
  const primaryIds = new Set([
    "base-homeschool-nheri-mid",
    "core-public-grades-5-8",
    "premier-nais",
    "premier-private-15k-plus",
  ]);
  const layers = marketLayers.filter((l) => primaryIds.has(l.id));

  return (
    <div className="section-extras">
      <CaveatNote label="Do not sum">
        Overlapping expansion layers. Scenarios are not SAM, forecasts, or current revenue. Each
        layer is an independent ceiling with its own source year and filters still required for SAM.
      </CaveatNote>

      <RuledGrid columns={2}>
        {layers.map((layer) => {
          const equation = `${formatLearners(layer.learners)} learners × $${layer.annualRevenuePerLearner.toLocaleString()}/yr = ${formatBillions(layer.modeledCeiling)}`;
          return (
            <RuledCard
              key={layer.id}
              footer={
                <div className="loop-diagram__footer">
                  <SystemLabel>
                    {layer.sourceYear} · {layer.observedOrModeled}
                  </SystemLabel>
                  <SystemLabel tone="attention">Not additive</SystemLabel>
                </div>
              }
            >
              <SystemLabel tone="accent">{layer.package.toUpperCase()}</SystemLabel>
              <p className="type-h3" style={{ margin: 0 }}>
                {formatBillions(layer.modeledCeiling)}
              </p>
              <p className="type-small" style={{ margin: 0 }}>
                {layer.segment}
              </p>
              <p className="type-label text-muted" style={{ margin: 0 }}>
                {equation}
              </p>
              <CaveatNote>{layer.caveat}</CaveatNote>
              <SourceLinkList sources={resolveSources(layer.sourceIds)} />
            </RuledCard>
          );
        })}
      </RuledGrid>

      <p className="chart-summary">
        Four vertical expansion layers · additive flag always false · no automatic total rendered
      </p>
    </div>
  );
}
