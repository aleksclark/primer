# LMS enhancement roadmap for workstation-delivered courses

This roadmap turns the verified gaps from `content/technology/basic_linux/PLATFORM_GAPS.md` into dependency-ordered implementation phases. The Linux course is the first demanding consumer, but the capabilities are intentionally general: curated instruction, trustworthy continuous evidence, mastery-gated sequencing, offline delivery, and portfolio projects apply across Primer.

## Governing principles

1. **The parent remains the primary educator.** Primer may collect evidence, enforce deterministic requirements, and recommend a next activity. It does not replace parental judgment for oral explanation, project quality, safety, or final mastery decisions.
2. **Mastery is evidence-based, not calendar-based.** Course order defines prerequisites and opportunities, not dates. The overseer advances a student only when eligibility and evidence policy permit it.
3. **The student does real work.** Checks should prove outcomes and craftsmanship without forcing one exact command or rewarding canned text.
4. **Continuous evidence is not formal assessment.** A paired workstation can provide useful procedural evidence, but cannot prove an uncompromised environment or independently establish conceptual mastery.
5. **Curated content is immutable and reviewable.** Parent-selected instruction, standards, tasks, and policies are versioned together. Generated tutoring operates within that authoritative revision.
6. **Offline operation must be honest.** Previously downloaded work remains usable, local checks remain deterministic, and unsynced or unreviewed evidence is labeled accurately.
7. **Security boundaries stay narrow.** Runtime closures are allowlisted, artifacts are bounded, activity documents cannot execute arbitrary verifier code, and imports are authenticated and auditable.

## Verified current-state corrections

The original gap list was directionally useful but several claims needed refinement:

- Primer already has an overseer and reinforcement-aware `assign-next`; it lacks course membership, prerequisite eligibility, and parent-controlled sequencing rather than all sequencing.
- Generic standards CRUD exists. The missing operational capability is a guarded, document-level import that resolves subject codes and reconciles only authorized custom standards.
- Work responses already carry changed assignment states. The client needs durable cursors, full pagination, and snapshot recovery; separate deletion tombstones are unnecessary while assignments remain soft-cancelled.
- Lesson-to-lesson artifact promotion is valuable for deliberate portfolio projects, but a clean fixture is often the more reproducible assessment. Promotion remains optional and parent-approved.
- `whoami` and `id` may already exist through the full coreutils closure. Runtime work begins with a closure/manifest audit rather than assuming every named command is absent.
- The most immediate undisclosed blocker is PTY evidence quality: the current PTY path synthesizes a successful `pty-shell` observation from the visible screen rather than receiving a trustworthy completed-command event.

## Phases

| Phase | Plan | Outcome |
|---|---|---|
| 0 | [Truthful evidence and mastery policy](phase-0-truthful-evidence-and-mastery.md) | UAT cannot overclaim command or conceptual mastery |
| 1 | [Contract and authoring foundations](phase-1-contract-and-authoring-foundations.md) | One strict, versioned activity contract with reliable validation |
| 2 | [Trustworthy terminal evidence](phase-2-trustworthy-terminal-evidence.md) | PTY sessions emit structured command and workspace evidence |
| 3 | [Instruction and human-reviewed responses](phase-3-instruction-and-human-review.md) | Rich curated teaching and conceptual evidence reach the student and parent |
| 4 | [Mastery-gated course orchestration](phase-4-mastery-gated-orchestration.md) | Existing overseer advances students through parent-approved course sequences |
| 5 | [Offline reconciliation and runtime capabilities](phase-5-offline-and-runtime-readiness.md) | Long courses remain correct offline and run against declared tool closures |
| 6 | [Portfolio continuity and curriculum operations](phase-6-portfolio-and-import-operations.md) | Selected projects persist safely and curriculum deployment is resumable and auditable |

## Dependency order

Phases are ordered by integrity rather than feature visibility. Phase 0 limits false claims immediately and reserves evidence vocabulary without building response review. Phase 1 stabilizes the contract before new evidence and teaching types are added. Phase 2 makes terminal observations trustworthy. Phase 3 owns response persistence, tutor/guard evaluation, and parent review. Phase 4 owns course persistence and eligibility while reusing existing curriculum/enrollment models. Phase 5 replaces the temporary capability gate and makes the sequence reliable on deployed workstations. Phase 6 owns artifact bytes/promotion and full bundle plan/apply operations.

A later phase may be prototyped early, but no phase should declare its exit criteria met while a dependency remains incomplete. When a phase introduces a temporary compatibility surface, the plan names the later phase that replaces it; parallel sources of truth are not retained.

## Course UAT while the roadmap is incomplete

Use the Linux course as a supervised reference workload:

1. Publish content without bulk assignment.
2. Assign lessons 1, 6, 13, 16, and 19 one at a time.
3. Treat completion as procedural session evidence only.
4. Require parent review for conceptual claims and capstone quality.
5. Do not accept standards mastery from command-sensitive checks until Phase 2 passes physical PTY acceptance.
6. Prefer the clean Lesson 20 fixture over cross-lesson artifact transfer until Phase 6.
