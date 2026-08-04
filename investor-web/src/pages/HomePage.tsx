import { InvestorCTA, SectionFrame } from "@/components";
import {
  CompetitionSection,
  CurrentStateSection,
  EvidenceSection,
  FounderSection,
  GtmSection,
  MarketSection,
  ProblemSection,
  ProductSection,
  RoundSection,
  TeachingSection,
  ThesisSection,
} from "@/components/sections";
import { sections, sources } from "@/data";
import type { ReactNode } from "react";

function sourcesFor(citationIds: string[]) {
  return sources.filter((s) => citationIds.includes(s.id));
}

const sectionBodies: Record<string, () => ReactNode> = {
  thesis: () => <ThesisSection />,
  problem: () => <ProblemSection />,
  product: () => <ProductSection />,
  "how-it-teaches": () => <TeachingSection />,
  proof: () => <EvidenceSection />,
  market: () => <MarketSection />,
  "go-to-market": () => <GtmSection />,
  competition: () => <CompetitionSection />,
  founder: () => <FounderSection />,
  "current-state": () => <CurrentStateSection />,
  round: () => <RoundSection />,
};

/**
 * Primary investor page — complete static thesis-to-ask argument.
 * All claims and statuses come from structured data modules.
 */
export function HomePage() {
  const total = sections.length;

  return (
    <>
      {sections.map((section, i) => {
        const body = sectionBodies[section.id];
        return (
          <SectionFrame
            key={section.id}
            section={section}
            index={i + 1}
            total={total}
            sources={sourcesFor(section.citationIds)}
          >
            {body ? body() : null}
          </SectionFrame>
        );
      })}

      <div className="site-container" style={{ paddingBottom: "var(--primer-space-12)" }}>
        <InvestorCTA
          eyebrow="Contact"
          headline="Ready to discuss the pre-seed."
          body="Working target: $3.5M pre-seed for 18 months of seed-readiness work. No student data in demos — synthetic artifacts only."
          primaryLabel="Discuss the pre-seed"
          primaryHref="#contact"
          secondaryLabel="View product state"
          secondaryHref="/demo"
        />
      </div>
    </>
  );
}
