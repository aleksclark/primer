# Investor-pitch website

This directory defines a scroll-driven investor website for Primer. It is a narrative specification, not production copy or implementation. The site should let an investor understand the company in five minutes and investigate sources, assumptions, product state, and risks without leaving the page.

## Objective

Move the reader through one argument:

> Individual attention works, but does not scale through one adult. Primer is an instructional LMS that applies the adult's standards continuously, records the evidence behind mastery, and can enter the market through families before expanding into schools.

## Audience

Primary:

- pre-seed and seed investors in education, AI application software, future of work/family, and vertical SaaS;
- education operators and potential advisors evaluating the founder and thesis.

Secondary:

- design-partner families;
- microschool and school leaders;
- prospective curriculum, research, and distribution partners.

The website is written for investors. Family and school calls to action should remain subordinate.

## Content principles

1. **Lead with the problem, not AI.** Generic tutoring is becoming cheap; persistent educational judgment, longitudinal evidence, and adult governance are the product.
2. **Separate current product from vision.** Label every module `LIVE`, `IN DEVELOPMENT`, or `PLANNED`.
3. **Separate evidence types.** Research supporting tutoring is not Primer efficacy; student records are not company outcomes.
4. **Use bottom-up markets.** Show learners × annual price with visible assumptions.
5. **Be candid about stage.** Approximately 40 founder hours and $1,000 invested, one week of initial family use, and no retention or learning-outcome claims yet.
6. **Make the founder story concrete.** Homeschool student, homeschool parent, current parent-builder, and former LMS engineer.
7. **Show school expansion as additive.** Families buy first; schools can pilot Primer supplementarily through LTI and SSO; primary-LMS status is earned through adoption.
8. **Use artifacts over adjectives.** Tutor exchanges, mastery records, project artifacts, integration diagrams, and product screenshots should carry the argument.
9. **Keep caveats adjacent.** Every modeled number or planned capability receives an inline label or footnote.
10. **Do not hide the proof plan.** The route from prototype to independently measured efficacy is part of the investment case.

## Files

- [narrative.md](narrative.md): the core argument and section sequence
- [site-map.md](site-map.md): page/section architecture and interaction model
- [content-spec.md](content-spec.md): copy, visual, data, and evidence requirements per section
- [diligence.md](diligence.md): deeper material linked from the primary narrative
- [implementation/](implementation/README.md): phased build plan using the System C design system

## Recommended experience

Use a single primary scrolling page with anchored navigation and optional deep-dive routes. The main page should remain coherent without opening a modal or appendix. Deeper market models, citations, compliance roadmaps, and founder details can live in dedicated diligence pages.

### Primary navigation

- Thesis
- Product
- Market
- Evidence
- Company
- Ask

### Persistent controls

- `View product` opens a short, truthful demo sequence.
- `Read sources` jumps to citations/diligence.
- `Investor access` can later gate detailed financials or a data room.
- A product-state legend explains `LIVE`, `IN DEVELOPMENT`, and `PLANNED`.

## Conversion goal

Primary call to action:

**Discuss the seed round**

Secondary calls to action:

- View the product
- Review the evidence plan
- Explore the market model

Do not use consumer purchase calls to action on the investor version.

## Blocking inputs before external publication

- validate the working $3.5 million pre-seed budget for an 18-month, three-person senior team;
- validate the month-18 seed-readiness scorecard: 1,500 paying learners, $900K ARR run rate, 85% term retention, independent learning progression, and 70% post-credit gross margin;
- show the ownership mandate and milestones attached to each $300,000 senior role;
- define included usage and COGS envelopes for Base $50, Core $100, and Premier $300;
- establish a pilot design and independent education/research reviewer;
- collect enough usage to report activation and retention honestly;
- select a launch subject and state-aligned assessment;
- produce actual product artifacts for the tutoring and mastery loop;
- verify all founder career claims and identify any usable eBackpack recognition;
- complete brand/trademark diligence against the existing Primer microschool company;
- decide which financial and architecture details require gated access;
- present financing dilution and the focused employee option pool together.
