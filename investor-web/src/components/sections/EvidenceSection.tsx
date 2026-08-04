import { EvidenceExplorer } from "@/components/explorers";

/**
 * Three-tier evidence with expandable research claims.
 * Company tier remains visually distinct and NOT YET MEASURED.
 */
export function EvidenceSection() {
  return (
    <div className="section-extras">
      <EvidenceExplorer />
    </div>
  );
}
