import { useMemo, useState } from "react";
import { CaveatNote } from "@/components/CaveatNote";
import { PrimaryButton } from "@/components/PrimaryButton";
import { RuledCard } from "@/components/RuledCard";
import { SourceLinkList } from "@/components/SourceLink";
import { SystemLabel } from "@/components/SystemLabel";
import { marketLayers, sources } from "@/data";
import type { MarketLayer } from "@/data/types";
import { formatCeiling, formatLearners } from "@/lib/format";
import {
  defaultsFromLayer,
  recalculateLayer,
  tryCombineLayerCeilings,
  type LayerInputs,
} from "@/lib/marketMath";

const PRIMARY_IDS = [
  "base-homeschool-nheri-mid",
  "core-public-grades-5-8",
  "premier-nais",
  "premier-private-15k-plus",
] as const;

function resolveSources(ids: string[]) {
  return sources.filter((s) => ids.includes(s.id));
}

function buildDefaultMap(layers: MarketLayer[]): Record<string, LayerInputs> {
  return Object.fromEntries(layers.map((l) => [l.id, defaultsFromLayer(l)]));
}

/**
 * Interactive family expansion explorer.
 * Static layer cards remain visible without JS interaction semantics changing defaults.
 */
export function MarketExplorer() {
  const layers = useMemo(
    () => marketLayers.filter((l) => (PRIMARY_IDS as readonly string[]).includes(l.id)),
    [],
  );
  const [inputs, setInputs] = useState<Record<string, LayerInputs>>(() => buildDefaultMap(layers));
  const [copyState, setCopyState] = useState<string | null>(null);

  const computed = layers.map((layer) =>
    recalculateLayer(layer, inputs[layer.id] ?? defaultsFromLayer(layer)),
  );
  const combine = tryCombineLayerCeilings(computed);
  const anyDirty = computed.some((c) => c.dirty);

  function updateLayer(id: string, patch: Partial<LayerInputs>) {
    setInputs((prev) => {
      const base = prev[id] ?? defaultsFromLayer(layers.find((l) => l.id === id)!);
      return {
        ...prev,
        [id]: {
          learners: patch.learners ?? base.learners,
          annualRevenuePerLearner:
            patch.annualRevenuePerLearner ?? base.annualRevenuePerLearner,
        },
      };
    });
  }

  function resetAll() {
    setInputs(buildDefaultMap(layers));
    setCopyState(null);
  }

  async function copyEquation(equation: string, id: string) {
    try {
      await navigator.clipboard.writeText(equation);
      setCopyState(id);
      window.setTimeout(() => setCopyState((cur) => (cur === id ? null : cur)), 1500);
    } catch {
      setCopyState("failed");
    }
  }

  return (
    <div className="explorer" data-explorer="market">
      <div className="explorer__toolbar">
        <div className="explorer__toolbar-text">
          <SystemLabel tone="accent">Market expansion explorer</SystemLabel>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            Adjust learners and annual ARPU per vertical layer. Combined totals across overlapping
            populations stay disabled.
          </p>
        </div>
        <PrimaryButton variant="secondary" onClick={resetAll} disabled={!anyDirty}>
          Reset to sourced defaults
        </PrimaryButton>
      </div>

      <CaveatNote label="Do not sum">
        {combine.reason} Additive flag is always false.
      </CaveatNote>

      <div className="explorer-layer-list" role="list">
        {layers.map((layer, index) => {
          const result = computed[index];
          const value = inputs[layer.id] ?? defaultsFromLayer(layer);
          const equation = result.equation;
          return (
            <RuledCard
              key={layer.id}
              selected={result.dirty}
              className="explorer-layer"
              as="div"
              footer={
                <div className="loop-diagram__footer">
                  <SystemLabel>
                    {layer.sourceYear} · {result.observedOrModeled}
                    {result.dirty ? " (edited)" : ""}
                  </SystemLabel>
                  <SystemLabel tone="attention">additive: false</SystemLabel>
                </div>
              }
            >
              <div className="explorer-layer__head">
                <SystemLabel tone="accent">{layer.package.toUpperCase()}</SystemLabel>
                <p className="type-h3" style={{ margin: 0 }} aria-live="polite">
                  {formatCeiling(result.modeledCeiling)}
                </p>
              </div>
              <p className="type-small" style={{ margin: 0 }}>
                {layer.segment}
              </p>

              <div className="explorer-controls">
                <label className="explorer-field" htmlFor={`learners-${layer.id}`}>
                  <span className="system-label">Learners</span>
                  <input
                    id={`learners-${layer.id}`}
                    type="number"
                    inputMode="numeric"
                    min={0}
                    step={layer.learners >= 1_000_000 ? 10_000 : 1_000}
                    value={value.learners}
                    onChange={(e) =>
                      updateLayer(layer.id, {
                        learners: Number(e.target.value),
                      })
                    }
                    aria-describedby={`learners-hint-${layer.id}`}
                  />
                  <span id={`learners-hint-${layer.id}`} className="type-small text-muted">
                    Default {formatLearners(layer.learners)}
                  </span>
                </label>

                <label className="explorer-field" htmlFor={`arpu-${layer.id}`}>
                  <span className="system-label">Annual ARPU (USD)</span>
                  <input
                    id={`arpu-${layer.id}`}
                    type="number"
                    inputMode="numeric"
                    min={0}
                    step={50}
                    value={value.annualRevenuePerLearner}
                    onChange={(e) =>
                      updateLayer(layer.id, {
                        annualRevenuePerLearner: Number(e.target.value),
                      })
                    }
                    aria-describedby={`arpu-hint-${layer.id}`}
                  />
                  <span id={`arpu-hint-${layer.id}`} className="type-small text-muted">
                    Default ${layer.annualRevenuePerLearner.toLocaleString("en-US")}
                  </span>
                </label>
              </div>

              <div className="explorer-equation">
                <p className="type-label text-muted" style={{ margin: 0 }} aria-live="polite">
                  {equation}
                </p>
                <PrimaryButton
                  variant="quiet"
                  onClick={() => void copyEquation(equation, layer.id)}
                  aria-label={`Copy equation for ${layer.package}`}
                >
                  {copyState === layer.id ? "Copied" : "Copy equation"}
                </PrimaryButton>
              </div>

              <CaveatNote>{layer.caveat}</CaveatNote>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                Overlap group · {layer.overlapGroup}
              </p>
              <SourceLinkList sources={resolveSources(layer.sourceIds)} />
            </RuledCard>
          );
        })}
      </div>

      <div
        className="explorer-warning"
        role="status"
        aria-live="polite"
      >
        <SystemLabel tone="attention">Combined total</SystemLabel>
        <p className="type-small" style={{ margin: 0 }}>
          Disabled · {combine.reason}
        </p>
      </div>

      <div className="explorer-table-wrap" role="region" aria-label="Expansion layer results table">
        <table className="explorer-table">
          <thead>
            <tr>
              <th scope="col">
                <SystemLabel>Layer</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Learners</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>ARPU</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Ceiling</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Source</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>State</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Additive</SystemLabel>
              </th>
            </tr>
          </thead>
          <tbody>
            {computed.map((row, i) => (
              <tr key={row.id} data-dirty={row.dirty ? "true" : "false"}>
                <th scope="row" className="type-small">
                  {layers[i].package.toUpperCase()} · {layers[i].segment}
                </th>
                <td className="type-small">{row.learners.toLocaleString("en-US")}</td>
                <td className="type-small">
                  ${row.annualRevenuePerLearner.toLocaleString("en-US")}
                </td>
                <td className="type-small">{formatCeiling(row.modeledCeiling)}</td>
                <td className="type-small">
                  {row.sourceYear}
                </td>
                <td className="type-small">{row.observedOrModeled}</td>
                <td className="type-small text-attention">false</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <p className="chart-summary">
        Four vertical expansion layers · additive:false · no automatic total · reset restores sourced
        defaults
      </p>
    </div>
  );
}
