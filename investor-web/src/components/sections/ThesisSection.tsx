import { MetricBlock, MetricRow } from "@/components/MetricBlock";
import { SystemLabel } from "@/components/SystemLabel";

/** Hero extras: positioning strip without outcome claims. */
export function ThesisSection() {
  return (
    <div className="section-extras">
      <MetricRow summary="Positioning summary for Primer investor thesis">
        <MetricBlock label="Product class" value="LMS" hint="Instructional, not administrative" />
        <MetricBlock label="Grades" value="5–8" hint="Family first, school ready" />
        <MetricBlock label="Entry" value="Family" hint="Supplementary school path" />
      </MetricRow>
      <SystemLabel>Base · Core · Premier responsibility ladder — detail in model</SystemLabel>
    </div>
  );
}
