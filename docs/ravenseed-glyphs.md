# RavenSeed Glyphs

RavenSeed Glyphs is the WebUI's stable icon and brand-mark system.

## Status

Version 0.9 is the release-candidate stabilization pass. The geometry, canonical names, and React props are frozen unless a concrete usability defect is found before 1.0.

## Geometry

- Master grid: 24 × 24
- Default WebUI size: 20px
- Compact size: 16px
- Prominent size: 24px
- Functional stroke: 1.75
- Caps and joins: round
- Functional icons: outline-first
- Color: `currentColor`
- Raven brand mark: filled silhouette on a 128 × 128 master

## React API

Import named components from `web/src/components/icons.tsx`.

```tsx
import {
  CandidateIcon,
  ReleaseFamilyIcon,
  SearchIcon,
} from "./components/icons";

<SearchIcon size={20} />
<CandidateIcon size={16} className="muted" />
<ReleaseFamilyIcon aria-label="Release family" />
```

All icons accept normal SVG props plus:

```ts
type RavenSeedIconProps = {
  size?: number | string;
  title?: string;
  // ...normal SVG props
};
```

Unlabelled icons are decorative and receive `aria-hidden="true"`. Add `title` or `aria-label` only when the SVG itself carries accessible meaning. When an icon appears inside a labelled button or link, keep the icon decorative.

## Canonical names

The canonical programmatic names are exported through `iconRegistry` and the `RavenSeedIconName` type. They are kebab-case semantic names such as:

- `search`
- `close`
- `search-history`
- `settings`
- `indexer`
- `target`
- `candidate`
- `release-family`
- `matching-rules`
- `metadata`
- `monitor`
- `plus`
- `more`
- `spinner`
- `raven-mark`

Do not create alternate aliases for the same concept without updating the design system.

## Semantic rules

Use `TargetIcon` for the selected destination indexer. Use `CandidateIcon` for a release family that may be missing from that target. Use `ReleaseFamilyIcon` for grouped equivalent releases. Use `IndexerIcon` for an indexer or indexer collection. Use `MatchingRulesIcon` for release-matching configuration. Use `MonitorIcon` for tracker monitors (not `SearchHistoryIcon`, which means past searches). Use `PlusIcon` for create actions and `MoreIcon` for an overflow menu trigger.

Keep these meanings stable. Do not substitute visually similar symbols merely because they fit a layout.

## Loading

Use `SpinnerIcon` with the existing `spin` class for indeterminate activity.

```tsx
<SpinnerIcon size={16} className="spin" />
```

## Brand assets

Two browser-ready assets live in `web/public`:

- `favicon.svg` — dark rounded tile with the white raven
- `ravenseed-mark.svg` — standalone white raven on transparent background

The React equivalent is `RavenMarkIcon`.

## Raven reactions

The raven mark also appears as a small character in `components/raven-reactions.tsx`. Each reaction sits on the thing it reacts to, never blocks a click, and is decorative (`aria-hidden`) except for any real button it carries:

- **Peek** (`RavenPeek`, mounted once in the app layout): perches on a focused password or `SecretInput` field, follows the caret, and covers its eyes while the value is shown. URL fields (`data-url-field`) are ignored.
- **xD** (`RavenError`): a 40px crossed-eye raven inside error notices; the same message returning within two minutes escalates to xDD and xDDD.
- **got coke?** (`useRavenReaction().show("opportunities", …)`): hops onto the element that holds the new candidates and offers a "show them" button.
- **spread the seed** (`SeedProgress`): the raven flies along a search's progress line and drops one seed per indexer as it answers.

With `prefers-reduced-motion`, reactions appear in place without animation.

## QA

The internal route `/dev/icons` renders the canonical registry at 16, 20, and 24px. Check this page after adding or modifying any glyph.

Before changing an existing icon:

1. verify the issue in the real UI,
2. check 16 / 20 / 24px,
3. keep the established semantic meaning,
4. run frontend formatting, tests, typecheck/build.

## v1.0 checklist

- [x] Real-instance visual validation
- [x] Native RavenSeed brand mark in the WebUI
- [x] Native loading glyph
- [x] No Lucide runtime dependency
- [x] Canonical typed icon registry
- [x] Browser favicon and standalone mark
- [x] Icon QA route
- [x] Accessibility tests
- [ ] Final CI green on the v0.9 release-candidate PR
- [ ] Tag/freeze RavenSeed Glyphs 1.0
