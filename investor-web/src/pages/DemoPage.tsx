import { DiligencePage, DiligenceSection } from "@/components/DiligencePage";
import { ProductStateExplorer } from "@/components/explorers/ProductStateExplorer";
import { DemoExplorer } from "@/components/explorers/DemoExplorer";
import { CaveatNote } from "@/components/CaveatNote";
import { DEMO_UPDATED, liveSurfaces, targetExperienceSteps } from "@/data/demoScript";
import { productState, sources } from "@/data";

const nav = [
  { id: "live-surfaces", label: "Live product surfaces" },
  { id: "target-experience", label: "Target instructional experience" },
  { id: "product-state", label: "Product-state inventory" },
  { id: "legend", label: "Status legend" },
];

const pageSources = sources.filter((s) =>
  ["product-state-internal", "founder-attested", "seed-readiness-internal"].includes(s.id),
);

/**
 * Truthful product demo: LIVE surfaces vs TARGET EXPERIENCE flow.
 * Deterministic script only — no live model calls, synthetic data only.
 */
export function DemoPage() {
  const liveCount = productState.filter((p) => p.status === "LIVE").length;

  return (
    <DiligencePage
      eyebrow="Product demo"
      title="Current product surfaces and target instructional flow"
      summary="Show what is live today, then the end-to-end remediation experience labeled TARGET EXPERIENCE until it is demonstrable. Synthetic learner data only. Deterministic script — no live model inference on this site."
      updated={DEMO_UPDATED}
      nav={nav}
      sources={pageSources}
      caveat="Live and target areas are visually and verbally separated. Do not read the scripted flow as shipping product."
      secondaryAction={{ label: "Full product state on pitch", href: "/#current-state" }}
    >
      <DiligenceSection
        id="live-surfaces"
        title="Live product surfaces"
        eyebrow={`${liveSurfaces.length} surfaces · ${liveCount} LIVE capabilities`}
      >
        <p className="type-small text-muted prose-measure" style={{ margin: 0 }}>
          command-teacher, PrimerTV, LMS core/admin, and Ultralogical are live foundations. Captures
          below are synthetic descriptions suitable for a public investor site.
        </p>
      </DiligenceSection>

      <DiligenceSection
        id="target-experience"
        title="Target instructional experience"
        eyebrow={`${targetExperienceSteps.length} scripted steps`}
      >
        <CaveatNote label="TARGET EXPERIENCE" blocker>
          Syllabus ingestion through scheduled transfer check is the intended teaching loop. It is
          labeled TARGET EXPERIENCE until the full path is demonstrable in product. The script is
          deterministic and reviewed for educational behavior.
        </CaveatNote>
      </DiligenceSection>

      <DemoExplorer />

      <DiligenceSection id="product-state" title="Product-state inventory" eyebrow="Filterable">
        <p className="type-small text-muted prose-measure" style={{ margin: 0 }}>
          Full capability inventory with LIVE / IN DEVELOPMENT / PLANNED labels. Legend remains
          visible under every filter.
        </p>
        <div style={{ marginTop: "var(--primer-space-5)" }}>
          <ProductStateExplorer />
        </div>
      </DiligenceSection>

      <DiligenceSection id="legend" title="How to read status" eyebrow="System C">
        <ul className="section-list">
          <li className="type-small">
            <strong>LIVE</strong> — demonstrable today with a synthetic or redacted artifact.
          </li>
          <li className="type-small">
            <strong>IN DEVELOPMENT</strong> — actively being built; not claimed as shipping.
          </li>
          <li className="type-small">
            <strong>PLANNED</strong> — on the roadmap; no implied near-term availability.
          </li>
          <li className="type-small">
            <strong>TARGET EXPERIENCE</strong> — instructional flow shown for diligence honesty,
            not as a live demo of finished product.
          </li>
        </ul>
      </DiligenceSection>
    </DiligencePage>
  );
}
