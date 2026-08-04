import { ProductStateExplorer } from "@/components/explorers";

/** LIVE / IN DEVELOPMENT / PLANNED inventory with optional facet filters. */
export function CurrentStateSection() {
  return (
    <div className="section-extras">
      <ProductStateExplorer />
    </div>
  );
}
