import {
  InstitutionalExplorer,
  MarketExplorer,
} from "@/components/explorers";

/**
 * Family expansion explorer + separate institutional explorer.
 * Static sourced defaults remain the initial render; no silent mutation.
 */
export function MarketSection() {
  return (
    <div className="section-extras">
      <MarketExplorer />
      <InstitutionalExplorer />
    </div>
  );
}
