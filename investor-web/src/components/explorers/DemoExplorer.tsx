import { useEffect, useId, useMemo, useRef, useState } from "react";
import { CaveatNote } from "@/components/CaveatNote";
import { PrimaryButton } from "@/components/PrimaryButton";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import {
  buildTargetTranscript,
  liveSurfaces,
  SYNTHETIC_LEARNER,
  targetExperienceSteps,
  type DemoStep,
} from "@/data/demoScript";
import { productState } from "@/data";
import { analytics } from "@/lib/analytics";
import { cn } from "@/lib/cn";

/**
 * Deterministic product demo: live surfaces + scripted TARGET EXPERIENCE flow.
 * No live model calls. Synthetic learner data only. Transcript stays in sync with steps.
 */
export function DemoExplorer() {
  const [stepIndex, setStepIndex] = useState(0);
  const [autoPlay, setAutoPlay] = useState(false);
  const startedRef = useRef(false);
  const completedRef = useRef(false);
  const step = targetExperienceSteps[stepIndex] ?? targetExperienceSteps[0];
  const transcriptId = useId();
  const transcript = useMemo(() => buildTargetTranscript(), []);

  useEffect(() => {
    if (!startedRef.current) {
      startedRef.current = true;
      analytics.demoStart();
    }
  }, []);

  useEffect(() => {
    const current = targetExperienceSteps[stepIndex];
    if (!current) return;
    analytics.demoStep(stepIndex + 1, current.id);
    if (stepIndex >= targetExperienceSteps.length - 1 && !completedRef.current) {
      completedRef.current = true;
      analytics.demoComplete();
    }
  }, [stepIndex]);

  useEffect(() => {
    if (!autoPlay) return;
    const id = window.setInterval(() => {
      setStepIndex((i) => {
        if (i >= targetExperienceSteps.length - 1) {
          setAutoPlay(false);
          return i;
        }
        return i + 1;
      });
    }, 4000);
    return () => window.clearInterval(id);
  }, [autoPlay]);

  function go(delta: number) {
    setAutoPlay(false);
    setStepIndex((i) =>
      Math.max(0, Math.min(targetExperienceSteps.length - 1, i + delta)),
    );
  }

  return (
    <div className="explorer" data-explorer="demo">
      <CaveatNote label="Synthetic data">
        All demo content uses {SYNTHETIC_LEARNER}. No family records, production
        transcripts, or live model inference appear on this page.
      </CaveatNote>

      <section className="demo-block" aria-labelledby="live-surfaces-heading">
        <div className="demo-block__header">
          <SystemLabel tone="accent">Live product surfaces</SystemLabel>
          <h3 id="live-surfaces-heading" className="type-h3" style={{ margin: 0 }}>
            What exists today
          </h3>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            Each surface is marked LIVE and links to a synthetic capture description — not a
            production tenant.
          </p>
        </div>

        <RuledGrid columns={2}>
          {liveSurfaces.map((surface) => {
            const capability = productState.find((p) => p.id === surface.capabilityId);
            return (
              <div key={surface.id} id={surface.anchor} className="demo-surface-anchor">
                <RuledCard
                  as="article"
                  footer={
                    <div className="loop-diagram__footer">
                      <StateBadge status="LIVE" />
                      <SystemLabel>Synthetic capture</SystemLabel>
                    </div>
                  }
                >
                  <SystemLabel tone="accent">{surface.name}</SystemLabel>
                  <p className="type-small" style={{ margin: 0 }}>
                    {surface.summary}
                  </p>
                  <div className="demo-capture">
                    <SystemLabel>Capture</SystemLabel>
                    <p className="type-small text-muted" style={{ margin: 0 }}>
                      {surface.capture}
                    </p>
                  </div>
                  <p className="type-small" style={{ margin: 0 }}>
                    <strong className="type-label">Proves · </strong>
                    {surface.proves}
                  </p>
                  {capability?.notes ? (
                    <CaveatNote label="Note">{capability.notes}</CaveatNote>
                  ) : null}
                </RuledCard>
              </div>
            );
          })}
        </RuledGrid>
      </section>

      <section
        className="demo-block demo-block--target"
        aria-labelledby="target-experience-heading"
      >
        <div className="demo-block__header">
          <SystemLabel tone="attention">Target experience</SystemLabel>
          <h3 id="target-experience-heading" className="type-h3" style={{ margin: 0 }}>
            Instructional flow (not yet live end-to-end)
          </h3>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            Deterministic script — repeatable, reviewed, no inference cost. Each step names the
            mechanism it is meant to prove once built.
          </p>
          <span className="target-badge" aria-label="Status: target experience">
            TARGET EXPERIENCE
          </span>
        </div>

        <div className="demo-script" aria-live="polite">
          <div className="demo-script__rail" role="tablist" aria-label="Demo steps">
            {targetExperienceSteps.map((s, i) => (
              <button
                key={s.id}
                type="button"
                role="tab"
                aria-selected={i === stepIndex}
                className={cn(
                  "demo-script__step-btn",
                  i === stepIndex && "demo-script__step-btn--active",
                )}
                onClick={() => {
                  setAutoPlay(false);
                  setStepIndex(i);
                }}
              >
                <span className="demo-script__step-num">
                  {String(s.order).padStart(2, "0")}
                </span>
                <span>{s.title}</span>
              </button>
            ))}
          </div>

          <RuledCard
            className="demo-script__stage"
            attention
            footer={
              <div className="loop-diagram__footer">
                <SystemLabel tone="attention">TARGET EXPERIENCE</SystemLabel>
                <SystemLabel>
                  Step {step.order} of {targetExperienceSteps.length}
                </SystemLabel>
              </div>
            }
          >
            <DemoStepPanel step={step} />
            <div className="demo-script__controls">
              <PrimaryButton
                variant="secondary"
                onClick={() => go(-1)}
                disabled={stepIndex === 0}
              >
                Previous
              </PrimaryButton>
              <PrimaryButton
                variant="secondary"
                onClick={() => go(1)}
                disabled={stepIndex >= targetExperienceSteps.length - 1}
              >
                Next
              </PrimaryButton>
              <PrimaryButton
                variant="quiet"
                onClick={() => setAutoPlay((v) => !v)}
                aria-pressed={autoPlay}
              >
                {autoPlay ? "Pause autoplay" : "Autoplay steps"}
              </PrimaryButton>
            </div>
          </RuledCard>
        </div>

        <details className="demo-transcript">
          <summary className="demo-transcript__summary">
            <SystemLabel>Accessible full transcript</SystemLabel>
            <span className="type-small text-muted">
              Synchronized with the {targetExperienceSteps.length} scripted steps
            </span>
          </summary>
          <pre id={transcriptId} className="demo-transcript__body">
            {transcript}
          </pre>
        </details>
      </section>
    </div>
  );
}

function DemoStepPanel({ step }: { step: DemoStep }) {
  return (
    <div className="demo-step-panel">
      <div className="demo-step-panel__meta">
        <SystemLabel tone="accent">{step.actor}</SystemLabel>
        <span className="target-badge">TARGET EXPERIENCE</span>
      </div>
      <h4 className="type-h3" style={{ margin: 0 }}>
        {step.title}
      </h4>
      <p className="type-body" style={{ margin: 0 }}>
        {step.script}
      </p>
      <dl className="detail-list">
        <div>
          <dt>
            <SystemLabel>Mechanism</SystemLabel>
          </dt>
          <dd className="type-small">{step.mechanism}</dd>
        </div>
        {step.artifact ? (
          <div>
            <dt>
              <SystemLabel>Synthetic artifact</SystemLabel>
            </dt>
            <dd className="type-small">{step.artifact}</dd>
          </div>
        ) : null}
      </dl>
    </div>
  );
}
