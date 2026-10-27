# ReelAnvil — Brand and UI Theme Directions (v1)

Status: approved foundational visual direction; implementation design tokens and component details remain to be finalized. ReelAnvil is the selected project name. Reference image: `ReelAnvil_Brand_UI_Concept_v1.png`.

## Approved visual direction

The user likes the full concept board: an anvil/film-reel identity, a polished Sonarr-like dark dashboard, expanded job details, media library, storage settings, and theme previews. Keep the look modern, clean, information-dense, and subtly Nordic/industrial rather than medieval or overtly Viking. Maintain practical readability and accessibility. The user typically favors pastel purple, blue, and black.

## Candidate theme variants to retain and explore

1. **Aurora (purple/blue)** — preferred proposed default: near-black midnight navy, luminous but soft lavender/violet and periwinkle highlights, frosty blue secondary accents, restrained copper/ember for selective states.
2. **Dark (default concept)** — restrained midnight/slate/steel, purple navigation and progress, warm ember for caution/active processing where semantically appropriate.
3. **Slate (minimal)** — quieter steel-blue and slate surfaces with minimal violet, low visual noise for long sessions.

Consider a fourth **Pastel Aurora** variant if the palette remains cohesive: softer lavender, powder blue, black/navy, low-glare selection and progress indicators. Treat as an experiment, not a committed feature.

## Identity and component ideas

- Abstract anvil + film reel mark; simplify for favicon, desktop icon, and sidebar. Keep monochrome variant.
- Contemporary, slightly angular typography; subtle Nordic cues in branding only, not body copy or controls.
- Dark layered surfaces with distinct but understated borders; compact cards, readable tables, high information density.
- Processing jobs should have clickable expandable stage details, actual progress only, elapsed/current-stage elapsed (no ETA), logs, safe actions, and clear blocked-storage state, as established in the build standards.
- No fake UI controls, fake percentages, or static placeholders in production.
- Hero illustration/mountains are optional marketing art, not a dashboard background.
- Copper/amber must not replace conventional accessible warning/status cues. Ensure contrast and do not convey status by color alone.

## Visual reference caveat

The generated image is a mood/concept board. Text, sample counts, stages, timings, storage values and percentages in the artwork are illustrative, not approved data or exact UI requirements. Functional decisions in the build standards remain authoritative.

## Project decision — 2026-10-27

The user approved this concept as the visual **foundation** for the full ReelAnvil application. Establish theme tokens and shared components at project inception rather than restyling a plain UI later. Aurora is the proposed default; preserve Dark, Slate, and experimental Pastel Aurora as planned alternatives. The concept image remains reference artwork, while build standards govern actual behavior and job reporting.
