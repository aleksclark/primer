import { useMemo, useState } from "react";
import { CaveatNote } from "@/components/CaveatNote";
import { PrimaryButton } from "@/components/PrimaryButton";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { SourceLinkList } from "@/components/SourceLink";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { institutionalScenarios, sources } from "@/data";
import { formatCeiling } from "@/lib/format";
import { computeInstitutionalProgramme } from "@/lib/marketMath";

function resolveSources(ids: string[]) {
  return sources.filter((s) => ids.includes(s.id));
}

type ScenarioInputs = Record<string, { learners: number; annualPriceUsd: number }>;

function defaultInputs(): ScenarioInputs {
  return Object.fromEntries(
    institutionalScenarios.map((s) => [
      s.id,
      { learners: s.defaultLearners, annualPriceUsd: s.defaultAnnualPriceUsd },
    ]),
  );
}

/**
 * Institutional contracting explorer — separate from family expansion.
 * Never offers a combined total with Base/Core/Premier.
 */
export function InstitutionalExplorer() {
  const scenarios = institutionalScenarios;
  const [inputs, setInputs] = useState<ScenarioInputs>(() => defaultInputs());
  const [activeId, setActiveId] = useState(scenarios[0]?.id ?? "");

  const rows = useMemo(
    () =>
      scenarios.map((s) => {
        const value = inputs[s.id] ?? {
          learners: s.defaultLearners,
          annualPriceUsd: s.defaultAnnualPriceUsd,
        };
        const result = computeInstitutionalProgramme(value.learners, value.annualPriceUsd);
        const dirty =
          value.learners !== s.defaultLearners ||
          value.annualPriceUsd !== s.defaultAnnualPriceUsd;
        return { scenario: s, value, result, dirty };
      }),
    [inputs, scenarios],
  );

  const anyDirty = rows.some((r) => r.dirty);

  function update(id: string, patch: Partial<{ learners: number; annualPriceUsd: number }>) {
    setInputs((prev) => {
      const cur = prev[id] ?? defaultInputs()[id];
      return {
        ...prev,
        [id]: {
          learners: patch.learners ?? cur.learners,
          annualPriceUsd: patch.annualPriceUsd ?? cur.annualPriceUsd,
        },
      };
    });
  }

  return (
    <div className="explorer" data-explorer="institutional">
      <div className="explorer__toolbar">
        <div className="explorer__toolbar-text">
          <SystemLabel tone="accent">Institutional explorer</SystemLabel>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            School-paid scenarios use a different contracting party. Do not combine with family
            subscription ceilings.
          </p>
        </div>
        <PrimaryButton
          variant="secondary"
          onClick={() => setInputs(defaultInputs())}
          disabled={!anyDirty}
        >
          Reset institutional defaults
        </PrimaryButton>
      </div>

      <CaveatNote label="Separate contracts">
        Public intervention, elite learning support, microschool academic core, and international
        school licenses stay off the family Base/Core/Premier map unless the payer and deduplication
        rule are explicit.
      </CaveatNote>

      <div className="explorer-tabs" role="tablist" aria-label="Institutional scenarios">
        {scenarios.map((s) => {
          const selected = s.id === activeId;
          return (
            <button
              key={s.id}
              type="button"
              role="tab"
              id={`inst-tab-${s.id}`}
              aria-selected={selected}
              aria-controls={`inst-panel-${s.id}`}
              tabIndex={selected ? 0 : -1}
              className="explorer-tabs__tab"
              data-active={selected ? "true" : "false"}
              onClick={() => setActiveId(s.id)}
              onKeyDown={(e) => {
                const idx = scenarios.findIndex((x) => x.id === activeId);
                if (e.key === "ArrowRight") {
                  e.preventDefault();
                  const next = scenarios[(idx + 1) % scenarios.length];
                  setActiveId(next.id);
                } else if (e.key === "ArrowLeft") {
                  e.preventDefault();
                  const next = scenarios[(idx - 1 + scenarios.length) % scenarios.length];
                  setActiveId(next.id);
                } else if (e.key === "Home") {
                  e.preventDefault();
                  setActiveId(scenarios[0].id);
                } else if (e.key === "End") {
                  e.preventDefault();
                  setActiveId(scenarios[scenarios.length - 1].id);
                }
              }}
            >
              {s.name}
            </button>
          );
        })}
      </div>

      {rows.map(({ scenario, value, result, dirty }) => {
        const selected = scenario.id === activeId;
        return (
          <div
            key={scenario.id}
            role="tabpanel"
            id={`inst-panel-${scenario.id}`}
            aria-labelledby={`inst-tab-${scenario.id}`}
            hidden={!selected}
            className="explorer-tabpanel"
          >
            <RuledCard
              selected={dirty}
              footer={
                <div className="loop-diagram__footer">
                  <SystemLabel>
                    {scenario.sourceYear} ·{" "}
                    {dirty ? "derived" : scenario.observedOrModeled}
                  </SystemLabel>
                  <SystemLabel tone="attention">additive: false</SystemLabel>
                </div>
              }
            >
              <div className="explorer-layer__head">
                <SystemLabel tone="accent">{scenario.buyer}</SystemLabel>
                <StateBadge status={scenario.status} />
              </div>
              <p className="type-h3" style={{ margin: 0 }} aria-live="polite">
                {formatCeiling(result.annualValueUsd)}
                <span className="type-small text-muted"> / year programme value</span>
              </p>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {scenario.description}
              </p>

              <div className="explorer-controls">
                <label className="explorer-field" htmlFor={`inst-learners-${scenario.id}`}>
                  <span className="system-label">{scenario.unitLabel}</span>
                  <input
                    id={`inst-learners-${scenario.id}`}
                    type="number"
                    inputMode="numeric"
                    min={scenario.minLearners}
                    max={scenario.maxLearners}
                    step={scenario.learnerStep}
                    value={value.learners}
                    onChange={(e) =>
                      update(scenario.id, { learners: Number(e.target.value) })
                    }
                  />
                </label>
                <label className="explorer-field" htmlFor={`inst-price-${scenario.id}`}>
                  <span className="system-label">Annual price (USD)</span>
                  <input
                    id={`inst-price-${scenario.id}`}
                    type="number"
                    inputMode="numeric"
                    min={scenario.minAnnualPriceUsd}
                    max={scenario.maxAnnualPriceUsd}
                    step={scenario.priceStepUsd}
                    value={value.annualPriceUsd}
                    onChange={(e) =>
                      update(scenario.id, { annualPriceUsd: Number(e.target.value) })
                    }
                  />
                </label>
              </div>

              <p className="type-label text-muted" style={{ margin: 0 }} aria-live="polite">
                {result.equation}
              </p>
              <CaveatNote>{scenario.caveat}</CaveatNote>
              <SourceLinkList sources={resolveSources(scenario.sourceIds)} />
            </RuledCard>
          </div>
        );
      })}

      <RuledGrid columns={2}>
        {rows.map(({ scenario, result }) => (
          <RuledCard
            key={`summary-${scenario.id}`}
            footer={<SystemLabel tone="attention">Not summed with family layers</SystemLabel>}
          >
            <SystemLabel>{scenario.name}</SystemLabel>
            <p className="type-small" style={{ margin: 0 }}>
              {formatCeiling(result.annualValueUsd)}
            </p>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              {result.learners.toLocaleString("en-US")} × $
              {result.annualPriceUsd.toLocaleString("en-US")}
            </p>
          </RuledCard>
        ))}
      </RuledGrid>

      <p className="chart-summary">
        Four institutional scenarios · separate contracting parties · no blend into family TAM
      </p>
    </div>
  );
}
