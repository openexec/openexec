# Design review 5/6 — accessibility against WCAG 2.2

**Date:** 2026-10-24. **Task:** `869ef98846b232de823226dc79ae8836` (fifth step
of the OpenExec `design-review` run; step 1 is
[DESIGN_REVIEW_INTENT_2026-10.md](DESIGN_REVIEW_INTENT_2026-10.md), step 2 is
[DESIGN_REVIEW_DRIFT_2026-10.md](DESIGN_REVIEW_DRIFT_2026-10.md), step 3 is
[DESIGN_REVIEW_INTERACTION_2026-10.md](DESIGN_REVIEW_INTERACTION_2026-10.md),
step 4 is [DESIGN_REVIEW_INTERFACE_2026-10.md](DESIGN_REVIEW_INTERFACE_2026-10.md)).

**Reviewed revision:** native `openexec` `origin/main`
`ebf32d3e4d8a17862a2d9c29fd2c649209037bf9`. Triage required at least
`0a6849bb` (step 4, #138). One commit landed after it, #139 (`ebf32d3e`), and it
changes only `internal/cli/chat_escalation_test.go`. The newest commit touching
`ui/src` or `internal/tui` is `0d937748` (#115, 2026-10-18). Every `path:line`
below is at `ebf32d3e`.

This report audits native OpenExec's two owner surfaces against WCAG 2.2
Level A and AA:
- the **web UI**: the React UI embedded in the daemon (`ui/src`, served at `/`);
- the **terminal UI**: `openexec tui` (`internal/tui`).

It changes no code, adds no dependency and raises no owner decision. It is an
evidence review, not a conformance claim or a certification. It is not evidence
of Goal Ready or of served behaviour.

## Rule being applied

- The step's contract, Agent Console `internal/playbook/playbook.go:279-286`,
  read from the local Console checkout: "Each check states whether it was
  machine-verified or read by a person. A snapshot attached for later reading
  is evidence, never a passed check." The step description is "Keyboard paths,
  focus management and return, contrast, semantic structure, screen reader
  output, touch targets, reduced motion. Run whatever the project automates,
  then check by hand what it does not."
- The baseline is **WCAG 2.2**, W3C Recommendation of 12 December 2024
  (<https://www.w3.org/TR/WCAG22/>), Level A and AA. Criterion wording quoted
  below was read from that page on 2026-10-24.
- The only declared accessibility clause for these surfaces is CR:387-393
  (`ui/src/components/chat/README.md`): "Full keyboard navigation", "ARIA
  labels on interactive elements", "Focus management on message send",
  "Screen reader announcements for events", "High contrast support for status
  indicators". The terminal UI has no accessibility clause; where a finding
  rests only on WCAG it says **no clause** for the native documents.
- No Agent Console style or design guide is used as a stand-in, and no
  product intent is inferred.

## Sources and labels

Labels N, K, CR and AG are step 4's (see its "Sources" table). New here:

| ID | Source |
|---|---|
| **W** | WCAG 2.2, W3C Recommendation 2024-12-12 |
| **PB** | Agent Console `internal/playbook/playbook.go:279-286` (step contract) |
| **CR** | `ui/src/components/chat/README.md`, last changed `96fa6397`, 2026-03-02; accessibility clause CR:387-393 |

**Check labels** (every check below carries one):
- **machine-verified**: an automated check that the repository itself runs
  (its vitest suite, lint, Go tests, declared checks) asserts the property.
- **executed by hand**: this pass ran something and observed the result: a
  temporary Playwright probe in Chromium (not committed, described under
  "Checks run"), or a contrast ratio computed from the colour values with the
  WCAG relative-luminance formula.
- **read from code**: inferred from source; nothing was run.

No snapshot or screenshot below counts as a pass. The probe's ARIA snapshots
are recorded as observations of what was exposed, not as passed checks.

**Classification and severity** are step 4's:
- **product**: code departs from the clause; **document**: the clause is stale,
  wrong or missing.
- **high**: defeats the surface's primary job for a user who relies on the
  criterion; **medium**: a user relying on the criterion is blocked or misled
  on a real path; **low**: a narrow path, a workaround exists, dead code, or
  an advisory item.

## 0. What the project automates for accessibility

**A0.1: No automated accessibility check exists. machine-verified absence is
not possible; this is read from code and confirmed by search.**
- `ui/eslint.config.js:1-5` imports only `@eslint/js`, `globals`,
  `eslint-plugin-react-hooks`, `eslint-plugin-react-refresh` and
  `typescript-eslint`. There is no `eslint-plugin-jsx-a11y`.
- `ui/package.json` devDependencies (`ui/package.json:26-47`) contain no axe
  package and no `jsx-a11y`. A search of `ui/package.json`,
  `ui/eslint.config.js` and `ui/package-lock.json` for `axe` or `jsx-a11y`
  returns only the unrelated `saxes` package.
- The vitest suite (jsdom) cannot compute layout, contrast or target size.
  18 test files use Testing Library role queries (`getByRole`/`findByRole`),
  which incidentally require an accessible name for the queried control. No
  unit test asserts focus position, focus return, live regions or reduced
  motion: a search of `ui/src` tests for `toHaveFocus`, `aria-live`,
  `prefers-reduced` or `toHaveAccessibleName` returns nothing.
- Classification: **product** (missing gate). Severity: **medium**. Per the
  contract this is reported, not added.

**A0.2: The generic Playwright suite's "Accessibility" tests assert nothing
about accessibility, and nothing runs them.**
- `ui/e2e/app.spec.ts:63-75` "should have accessible focus indicators" ends in
  `expect(hasFocusedElement).toBeGreaterThanOrEqual(0)`, which a count
  always satisfies (`:74`).
- `ui/e2e/multi-project-sessions.spec.ts:631-646` ends in
  `expect(hasFocused >= 0).toBe(true)` (`:645`).
  `:661-671` "should announce session changes to screen readers" counts live
  regions into `hasLiveRegion` and never asserts it; the only assertion is
  that `#root` is not empty (`:671`).
- `ui/e2e/provider-agnostic-loop.spec.ts:1137-1151` (`:1150`) and
  `:1166-1177` repeat both patterns for the model picker and provider status.
- The only real accessibility assertions in the suite are `lang="en"` and the
  viewport meta (`ui/e2e/app.spec.ts:78-87`).
- The generic suite is not run by `make test` (`Makefile:60-84` runs only the
  D4/D6 Chromium journeys) and `.github/workflows/ci.yml` has no Playwright
  step (a search for `playwright`/`e2e` in it returns nothing).
- Classification: **product** (vacuous tests). Severity: **low**: they give
  false assurance, though no gate relies on them. Read from code.

## 1. Web UI

The web UI has four views chosen by buttons in `ui/src/App.tsx:92-117`: Chat,
Knowledge Hub, Usage and Replay (Replay is a placeholder, `App.tsx:16`).
Executed-by-hand observations come from the temporary Chromium probe against
the Vite dev server with all `/api` calls mocked (no daemon, no model, no
streaming); see "Checks run".

### 1.1 Keyboard paths (2.1.1 Keyboard, A; 2.4.3 Focus Order, A)

W 2.1.1: "All functionality of the content is operable through a keyboard
interface". CR:389 "Full keyboard navigation".

**Pass (executed by hand):** On the Chat view with one session, Tab visits, in
order: the four nav buttons, "Collapse sidebar", the project combobox, "Init",
"Create new session", the session search, the filter toggle, the session row
(`role="button"`), the chat textarea and "Open panel", then leaves the page.
The order follows the visual layout (nav → sidebar → main → right panel). No
keyboard trap was met (2.1.2).

**WK1: A session's row actions appear only on mouse hover.**
- Evidence: `ui/src/components/chat/session/SessionListItem.tsx:109-110` sets
  `showActions` from `onMouseEnter`/`onMouseLeave` only; the buttons render
  only when `showActions` is true (`:138`). There is no `onFocus` handler.
- Executed by hand: with the row focused by keyboard the probe found 0
  `.session-list-item__action` buttons; after hovering, 1 ("Fork session"; the
  mounted page passes only `onFork`, `ui/src/pages/ChatPage.tsx:373`).
- Mitigation: forking the selected session is also offered by the header's
  fork button (`ui/src/components/chat/header/ChatHeader.tsx:143-147`,
  wired at `ChatPage.tsx:467`), so a keyboard user can select the row with
  Enter and fork from the header. Archive and delete are not wired on the
  mounted page.
- Criterion: 2.1.1. CR:389. Classification: **product**. Severity: **low**.

**WK2: Directory entries in the project-initialisation picker are clickable
`div`s.**
- Evidence: `ui/src/components/chat/session/DirectoryPicker.tsx:65-72`: a
  `div` with `onClick`, no role, no `tabIndex`, no key handler. It is mounted
  in `ProjectInitModal` (`ProjectInitModal.tsx:73`).
- Mitigation: the same dialog has a labelled path text field
  (`ProjectInitModal.tsx:61-63`), so the path can be typed.
- Criterion: 2.1.1, 4.1.2. Classification: **product**. Severity: **low**.
  Read from code.

**WK3: The new-session modal cannot be closed with Escape.**
- Executed by hand: after Enter on "Create new session" the
  `.new-session-modal` was present; after Escape it was still present (count
  1). Clicking the overlay closed it.
- Evidence: `ui/src/components/chat/session/SessionSidebar.tsx:204-205`
  renders the overlay with only `onClick`; the file has no `Escape` or key
  handler. `ProjectInitModal.tsx:41-42` and `ProjectWizard.tsx:121-122` use
  the same pattern (read from code).
- Criterion: no A/AA criterion requires Escape for a modal; it is a keyboard
  convention, and the form's own buttons remain reachable by Tab. Recorded
  under focus management (WF2) for the modal semantics. Classification:
  **product**. Severity: **low**. Label: **no clause** beyond CR:389.

### 1.2 Focus management and return (2.4.3 Focus Order, A; 2.4.7 Focus Visible, AA; 2.4.11 Focus Not Obscured (Minimum), AA)

W 2.4.3: "focusable components receive focus in an order that preserves
meaning and operability." W 2.4.7: "Any keyboard operable user interface has a
mode of operation where the keyboard focus indicator is visible."

**WF1: The fork dialog does not take focus, does not keep it, and does not
return it.**
- Evidence: `ui/src/components/chat/session/SessionForkDialog.tsx:213-221` and
  `:344-352` render `role="dialog"`, `aria-modal="true"`,
  `aria-labelledby="fork-dialog-title"` and `tabIndex={-1}` on the backdrop.
  Escape is handled by `onKeyDown` on that backdrop (`:188-195`). The file
  contains no `focus` call at all (search for `focus` returns nothing), so:
  - focus stays on the trigger outside the dialog when it opens;
  - Escape works only after the user has tabbed into the dialog, because the
    key handler is on the dialog's own element;
  - Tab is not contained (no trap), so focus can leave a modal dialog;
  - on close, nothing returns focus to the trigger.
- The accessible name and role are correct (`aria-labelledby` points at the
  `h2` at `:228` / `:359`).
- Not executed: the probe could not open it, because the header fork button
  appears only for a selected session with loaded history and the mocked API
  returned none (probe printed `FORK buttons 0`).
- Criterion: 2.4.3. CR:391 "Focus management". Classification: **product**.
  Severity: **medium**. Read from code.

**WF2: The three mounted modals that are not `SessionForkDialog` have no
dialog semantics and lose focus on close.**
- Executed by hand (new-session modal): after Enter on "Create new session"
  focus stayed on that button behind the modal, the page had 0
  `[role=dialog]` elements, and after the overlay closed the modal focus was on
  `BODY`.
- Read from code: `SessionSidebar.tsx:204-205`, `ProjectInitModal.tsx:41-42`
  and `ProjectWizard.tsx:121-122` are plain `div`s with no `role="dialog"`, no
  `aria-modal`, no label reference and no focus handling.
- Criterion: 2.4.3, 4.1.2 (role not exposed). Classification: **product**.
  Severity: **medium**.

**WF3: Unmounted dialogs repeat WF1's pattern.**
- `ui/src/components/chat/diff/PatchPreviewModal.tsx:121-143` and
  `ui/src/components/chat/diff/RollbackModal.tsx:96-119` have the same
  backdrop-only Escape handler and no `focus` call. `ModelPicker` moves focus
  into its search field on open (`ui/src/components/chat/header/ModelPicker.tsx:92-97`),
  closes on Escape (`:100-107`) and does not return focus to its trigger.
- None of the three is mounted by a page: step 4 U5 records the `diff/`
  components as unmounted, and `ModelPicker` is used only by
  `ModelPickerWithAvailability`, which no page renders (search for
  `<ModelPickerWithAvailability` returns nothing).
- Criterion: 2.4.3. Classification: **product**, dead surface. Severity:
  **low**. Read from code.

**WF4: `ApprovalsPanel` drops focus when the reject field closes or a card
resolves.**
- Evidence: the reject-reason field takes focus on open
  (`ui/src/components/chat/blueprint/ApprovalsPanel.tsx:372`) and Escape closes
  it (`:376-378`), unmounting the focused element with no focus target. A
  resolved approval is removed from the pending list (`:139`, `:259-269`), so
  a focused Approve button disappears the same way.
- Criterion: 2.4.3. Classification: **product**. Severity: **low**. Read from
  code; the D4 journey shows the panel renders but does not test focus.

**WF5: The focus indicator is removed from the chat input and 12 other
controls.**
- Evidence: `ui/index.html:36-39` sets a 2px `#58a6ff` `:focus-visible`
  outline, but inline `outline: 'none'` overrides it on: the chat textarea
  (`ui/src/components/chat/input/InputTextarea.tsx:124`), the project combobox
  (`ui/src/components/chat/session/ProjectSelector.tsx:125`), the session
  search (`ui/src/components/chat/session/SessionFilters.tsx:192`), and
  `SessionFilters.tsx:261`, `SessionSidebar.tsx:376`, `:387`,
  `ProjectInitModal.tsx:116`, `ProjectWizard.tsx:206`,
  `ToolCallApproval.tsx:172`, `SessionTitle.tsx:149`, `ModelPicker.tsx:496`,
  `TimeRangeSelector.tsx:319`, `BackupHistoryPanel.tsx:423`, `:450`. None of
  these files has an `onFocus` style replacement.
- Executed by hand: on keyboard focus the probe measured computed
  `outline: none 0px` for the project combobox, the session search and the
  chat textarea, and `solid 2px` for every button.
- Criterion: 2.4.7. CR:389. Classification: **product**. Severity:
  **medium**: the chat textarea is the view's primary control.

**WF6: The session filter toggle is covered by header text at a desktop
viewport.**
- Executed by hand: at Playwright's Desktop Chrome viewport (1280×720) the
  toggle's box was `{x:1240, y:63, 28×28}` and the topmost element at its
  centre was the header's `$0.0000` cost `span`; Playwright refused to click
  it ("intercepts pointer events"). Keyboard focus still reached it.
- Not established: whether the focused toggle is *entirely* hidden, which is
  the 2.4.11 test ("not entirely hidden due to author-created content"). Only
  the centre point was measured.
- Criterion: 2.4.11 (candidate), 2.5.8 (pointer). Classification: **product**.
  Severity: **low**.

### 1.3 Contrast (1.4.3 Contrast (Minimum), AA; 1.4.11 Non-text Contrast, AA)

W 1.4.3: "The visual presentation of text and images of text has a contrast
ratio of at least 4.5:1", 3:1 for large text. W 1.4.11: 3:1 "against adjacent
color(s)" for UI components and graphical objects. CR:393 "High contrast
support for status indicators".

**WC0: The colour tokens in `variables.css` are not applied. Contrast is
computed from the inline colours actually rendered.**
- `ui/src/styles/variables.css` (tokens at `:10-36`, `--touch-target-min:
  44px` at `:139`), `breakpoints.module.css` and `layout.module.css` are not
  imported anywhere: a search of `ui/src`, `ui/index.html` and
  `ui/vite.config.ts` for `.css` imports returns nothing, and no `.tsx` file
  uses `var(--`. All colours are inline `style` objects.
- Triage named `variables.css` as the contrast source; this corrects it.
- Classification: **document**/**product** (dead token file). Severity:
  **low**. Read from code.

All ratios below were **executed by hand** (computed with the WCAG 2.2
relative-luminance formula; the script is described under "Checks run").
Backgrounds: page `#0d1117`, sidebar and right panel `#161b22`
(`SessionSidebar.tsx:306`, `ChatLayout.tsx:169`, `:239`), chat input area
`#0d1117` (`ui/src/components/chat/layout/ChatMain.tsx:234`).

Pairs that pass, for the record: primary text `#c9d1d9` 12.26 / 11.21;
secondary `#8b949e` 6.15 / 5.62 (4.95 on `#21262d`); links and focus outline
`#58a6ff` 7.49 on `#0d1117`; white on `#238636` 4.63 (approve, running
badge); white on `#da3633` 4.61.

**WC1: Muted 11–12px text is below 4.5:1.**

| Where | Pair | Ratio |
|---|---|---|
| Chat input hint "Press Enter to send…" (`ui/src/components/chat/input/ChatInput.tsx:75`, style `:103-106`, 11px) | `#6e7681` on `#0d1117` | 4.12 |
| Session time (`SessionListItem.tsx:176`, style `:267-270`, 11px) and message count (`:293-296`) | `#6e7681` on `#161b22` | 3.77 |
| Events empty hint (`ui/src/components/chat/events/EventList.tsx:84`, style `:154-157`, 12px) | `#6e7681` on `#161b22` | 3.77 |
| Diff line numbers (`ui/src/components/chat/diff/DiffHunk.tsx:270`) — unmounted (step 4 U5) | `#484f58` on `#0d1117` | 2.28 |
| Model separator (`ui/src/components/chat/header/ModelIndicator.tsx:116`, mounted at `ChatHeader.tsx:124`) | `#484f58` on `#0d1117` | 2.28 |

`#6e7681` is used for text at 45 inline sites (search for
`color: '#6e7681'`); on `#21262d` it is 3.31. Criterion: 1.4.3.
Classification: **product**. Severity: **medium**.

**WC2: White text on orange and purple buttons and badges is below 4.5:1.**

| Where | Pair | Ratio |
|---|---|---|
| Loop badge "paused" (`ui/src/components/chat/header/LoopStatusBadge.tsx:35-37`) | `#ffffff` on `#f0883e` | 2.53 |
| Fork dialog primary button, mounted (`SessionForkDialog.tsx:974-977`, used `:330`, `:570`) | `#ffffff` on `#a371f7` | 3.35 |
| Rollback confirm button, unmounted (`RollbackModal.tsx:687-690`) | `#ffffff` on `#f0883e` | 2.53 |
| Assistant avatar icon (`ui/src/components/chat/messages/AssistantMessage.tsx:136-144`; `StreamingMessage.tsx:74-82`) | `#ffffff` on `#58a6ff` | 2.53 (graphic: 1.4.11 needs 3:1) |

Criterion: 1.4.3 (text), 1.4.11 (avatar icon). CR:393 asks for "High
contrast support for status indicators"; the paused badge contradicts it.
Classification: **product**. Severity: **medium**.

**WC3: Text-field boundaries are 1.55:1.**
- The chat textarea's only boundary is `1px solid #30363d` on `#0d1117`
  (`InputTextarea.tsx:120-121`): 1.55:1. The field background equals the page
  background, so the border is the only visual cue that a field is there.
- Criterion: 1.4.11 (W: "Visual information required to identify user
  interface components and states"). The placeholder text helps identify it,
  so this is a borderline case. Classification: **product**. Severity:
  **low**.

### 1.4 Semantic structure (1.3.1 Info and Relationships, A; 2.4.1 Bypass Blocks, A; 2.4.6 Headings and Labels, AA; 3.1.1 Language of Page, A)

**Passes:**
- `lang="en"` (`ui/index.html:2`) — machine-verified by
  `ui/e2e/app.spec.ts:78-82` only when the generic suite is run (A0.2);
  read from code here.
- The Chat view exposes `navigation`, `complementary` and `main` landmarks
  (`App.tsx:92`; `ui/src/components/chat/layout/ChatLayout.tsx:82`, `:101`,
  `:120`). Executed by hand: the ARIA snapshot showed them. This satisfies
  2.4.1 on Chat through landmarks.
- Dialog titles are headings with matching `aria-labelledby` (WF1).

**WS1: The active view is shown only visually.**
- Evidence: the active nav button gets a blue colour and 2px underline
  (`App.tsx:77-80`, applied `:94`, `:100`, `:106`, `:112`). No
  `aria-current`, `aria-pressed` or `aria-selected` is set.
- Executed by hand: the probe counted 0 such attributes in `nav`; the ARIA
  snapshot lists four plain buttons.
- Criterion: 1.3.1, 4.1.2. Classification: **product**. Severity: **medium**.

**WS2: Headings and landmarks differ by view; two views have no `main`.**
- Executed by hand (probe):

  | View | `main` | Headings |
  |---|---|---|
  | Chat, no session selected | 1 | H2 Sessions, H3 No Session Selected |
  | Knowledge Hub | 0 | H1 Project Knowledge Map, H2 ×2 |
  | Usage (API returning 503) | 1 | H1 Usage Dashboard, H3, H4, H3, H4 |
  | Replay | 0 | none |

- Chat gets an `h1` only once a session is selected
  (`ui/src/components/chat/header/SessionTitle.tsx:93`). Knowledge Hub's root
  is a `div` (`ui/src/pages/KnowledgeHub.tsx:95`). Replay is one `div` of text
  (`App.tsx:16`). The chat layout nests the sidebar's `aside`
  (`SessionSidebar.tsx:95`) inside another `aside` (`ChatLayout.tsx:82`); the
  snapshot shows `complementary` inside `complementary`, both unnamed.
- WCAG 2.2 A/AA does not require an `h1`, a `main`, or unskipped heading
  levels; these are **no clause** observations (advisory). Severity: **low**.

**WS3: Several form controls have no programmatic name or label.**
- Executed by hand (ARIA snapshot): the filter toggle is exposed as
  `button [expanded]` with no name
  (`ui/src/components/chat/session/SessionFilters.tsx:91-99`: icon only, no
  `aria-label`, no `title`). The Status and Sort comboboxes have no name: their
  `label` elements have no `htmlFor` and do not wrap the `select`
  (`SessionFilters.tsx:106-107`, `:121-122`).
- Read from code: the new-session modal's Title, Provider and Model labels are
  not associated with their controls (`SessionSidebar.tsx:211-212`,
  `:232-233`, `:256-257`); "Project Path" labels a `div` (`:223-226`). The
  Project Wizard's close button is named only by its "×" glyph
  (`ProjectWizard.tsx:125`).
- The session search, chat textarea and approval reject field are named only
  by placeholder (snapshot: `textbox "Search sessions..."`; `ApprovalsPanel.tsx:366-368`).
  That gives a name but no persistent visible label (3.3.2: "Labels or
  instructions are provided when content requires user input"); recorded as
  advisory.
- Criterion: 4.1.2, 1.3.1, 3.3.2. CR:390 "ARIA labels on interactive
  elements". Classification: **product**. Severity: **medium**.

**WS4: Session status is a coloured dot.**
- Evidence: `SessionListItem.tsx:121-127` draws an unlabelled dot whose colour
  comes from `getStatusColor` (`:92-99`: active green, paused orange, archived
  grey). The status text exists only in `title`. Executed by hand: assistive
  technology does receive it, since the row's accessible name begins "Status:
  active".
- Visually, status is carried by colour alone. Criterion: 1.4.1 ("Color is not
  used as the only visual means of conveying information"). CR:393.
  Classification: **product**. Severity: **low**.

### 1.5 Screen-reader output and announcements (4.1.3 Status Messages, AA; 4.1.2 Name, Role, Value, A)

W 4.1.3: "status messages can be programmatically determined through role or
properties" so they are presented "without receiving focus". CR:392 "Screen
reader announcements for events".

**WR1: Nothing in the chat UI is announced: no live region exists.**
- Read from code: the only `role="alert"` in `ui/src` is Knowledge Hub's
  error (`KnowledgeHub.tsx:100`). There is no `aria-live`, `role="status"` or
  `role="log"` in any `.tsx` file. So none of these is announced:
  - streamed assistant output and its "Thinking..." state
    (`ui/src/components/chat/messages/StreamingMessage.tsx:28-44`);
  - new messages appended to the list
    (`ui/src/components/chat/messages/MessageList.tsx:121-147`, no `role="log"`);
  - loop-status changes (`LoopStatusBadge.tsx`);
  - approval errors (`ApprovalsPanel.tsx:241-246`, a plain `div`);
  - new events in the right panel (`EventList.tsx`).
- Executed by hand: the probe counted 0 live regions in the Chat, Usage and
  Replay views and 0 in Knowledge Hub when no error showed.
- Context: step 4 U2 found that the daemon never sends the
  `streaming_chunk`/`message` events these components render, so against a
  real daemon there is no streamed text to announce yet. The missing live
  regions are independent of that and would apply once U2 is fixed.
- Criterion: 4.1.3. CR:392. Classification: **product**. Severity:
  **medium**.

**WR2: Decorative icons are exposed as unnamed images.**
- Executed by hand: the snapshot shows bare `img` nodes inside named buttons
  and beside "Search sessions...". The SVG icons carry no `aria-hidden`
  (e.g. `StreamingMessage.tsx:52-60`).
- Buttons that contain them are named, so 4.1.2 holds for the buttons; the
  stray images are noise. Criterion: 1.1.1 (advisory). Severity: **low**.

### 1.6 Touch and pointer targets (2.5.8 Target Size (Minimum), AA)

W 2.5.8: "The size of the target for pointer inputs is at least 24 by 24 CSS
pixels, except when:" (spacing, equivalent, inline, user agent control,
essential).

**Pass (executed by hand):** on the Chat view every visible button measured
at least 24×24 except those below; the icon buttons are 28×28 and the nav
buttons 34px tall. Session row actions are 24×24 by style
(`SessionListItem.tsx:323-328`, read from code; hover-only).

**WT1: Two buttons and one field are under 24px tall.**
- Executed by hand: "Init" 120×22 and "Wizard" 120×22
  (`ProjectSelector.tsx:58-72`, style `:96-102`, `padding: '4px 8px'`,
  `fontSize: '11px'`); the session search input 211×15
  (`SessionFilters.tsx:72`, style `:188-196`, `padding: 0`).
- The spacing exception was not measured. The search input sits in a padded
  container, but clicking the container does not focus the input.
- `--touch-target-min: 44px` exists only in the unloaded `variables.css`
  (WC0).
- Criterion: 2.5.8. Classification: **product**. Severity: **low**.

**Reflow (1.4.10, AA), executed by hand:** at a 320×640 viewport the Chat view
had `scrollWidth` 320 = `clientWidth` 320 (no horizontal scroll), in the
initial state only. Zoom is not disabled (`ui/index.html:6`).

### 1.7 Reduced motion (2.2.2 Pause, Stop, Hide, A; 2.3.3 Animation from Interactions, AAA — outside the A/AA baseline)

**WM1: The reduced-motion hook exists and nothing uses it.**
- Read from code: `usePrefersReducedMotion` is defined at
  `ui/src/hooks/useBreakpoint.ts:79-80`. It is not exported from
  `ui/src/hooks/index.ts` and a search of `ui/src`, `ui/index.html` and CSS
  for `usePrefersReducedMotion` or `prefers-reduced-motion` finds only the
  definition. Triage's expectation is confirmed.
- Motion that ignores the preference:
  - smooth auto-scroll on every new message and event
    (`MessageList.tsx:66`, `:74`, `:158`; `EventList.tsx:49`, `:57`, `:64`,
    `:133`);
  - infinite `spin` animations on loading icons (`index.html:30-33`; e.g.
    `ui/src/components/chat/layout/ChatMain.tsx:282`,
    `ui/src/components/chat/input/SendButton.tsx:60`, `ApprovalsPanel.tsx:440`).
- `pulse` and `blink` are referenced (`StreamingMessage.tsx:99`, `:118`;
  `LoopStatusBadge.tsx:129`; `EventList.tsx:194`) but no `@keyframes pulse` or
  `@keyframes blink` exists (only `spin`: `index.html:30`,
  `ProviderAvailabilityIndicator.tsx:236`, `:275`). Those elements therefore
  do not animate. Read from code; a browser would show it during streaming.
- Executed by hand: with `reducedMotion: 'reduce'` at 320×640 the matched
  media query was true and `document.getAnimations()` was empty in the idle
  state. That shows nothing about the streaming or loading states.
- Criterion: no A/AA criterion is failed by this evidence. Spinners shown only
  while loading are short-lived; 2.2.2 would apply to one that runs more than
  five seconds alongside other content, which was not observed. 2.3.3 is AAA.
  Classification: **product** (dead hook). Severity: **low**.

## 2. Terminal UI (`openexec tui`)

Native documents give the TUI no accessibility clause. Its only contract is
its help text (`internal/tui/help.go:18-32`) and command description
(`internal/cli/tui.go:13-14`). All TUI checks are **read from code**: the TUI
has no tests (`go test` prints `[no test files]`), and this pass did not run
the binary or a terminal screen reader.

**Which WCAG criteria carry over.** Bubble Tea writes a character grid to a
terminal, so markup-based criteria do not apply:

| Area | WCAG criterion | Applies to the TUI? |
|---|---|---|
| Keyboard paths | 2.1.1, 2.1.2 | Yes: the TUI is keyboard-only |
| Focus management and return | 2.4.3, 2.4.7 | By analogy: the selected row is the focus |
| Contrast | 1.4.3 | By analogy, for the colours the app sets; the terminal theme sets the rest |
| Colour only | 1.4.1 | Yes |
| Semantic structure | 1.3.1, 2.4.6 | Not applicable: a terminal has no headings or landmarks |
| Screen reader output | 4.1.2, 4.1.3 | Not applicable as written (no markup); not checkable here |
| Touch targets | 2.5.8 | Not applicable: no mouse mode (`internal/cli/tui.go:42` uses only `tea.WithAltScreen()`) |
| Reduced motion | 2.2.2 | Partly: the screen auto-refreshes every second |

### 2.1 Keyboard paths

**Pass:** every function is a key (`internal/tui/app.go:181-224`): q/Ctrl-C,
up/k, down/j, Enter, Esc, 1–9, g, G.

**TK1: Documented keys do nothing.**
- `?` "Toggle help" is empty (`app.go:205-206`); this is step 4 TU2 and is
  referenced, not repeated.
- New here: `g` and `G` in the log view set `scrollPosition`
  (`app.go:215-223`), but `viewLogs` never reads it; it always shows the tail
  of the buffer (`app.go:135-142`). The log footer advertises "[g] Top | [G]
  Bottom" (`app.go:144`; `help.go:42`).
- Criterion: **no clause** in WCAG A/AA (the keys exist and fail silently);
  the clause is the TUI's own help text. Classification: **product**.
  Severity: **low**.

### 2.2 Focus management and return

**Pass:** the selected project is marked with "▶ " (`app.go:153-157`) as well
as a background colour (`app.go:168-170`), so it remains visible without
colour. Esc returns from the log view to the dashboard without resetting
`selectedIdx` (`app.go:201-203`), so the selection is kept.

### 2.3 Contrast and colour

Colours are xterm 256-colour indices (`internal/tui/styles.go`), computed with
their standard xterm RGB values (39 `#00afff`, 255 `#eeeeee`, 59 `#5f5f5f`,
246 `#949494`, 46 `#00ff00`, 226 `#ffff00`, 42 `#00d787`, 196 `#ff0000`).
Terminal emulators may remap the palette, and the background is the user's
theme, so ratios are given against black and white. Executed by hand.

**TC1: The selected row is 2.11:1 whatever the terminal theme.**
- `styles.go:25-29` sets both colours: foreground 255 on background 39.
  `#eeeeee` on `#00afff` is 2.11:1. The row is bold but not large.
- Criterion: 1.4.3 by analogy. Classification: **product**. Severity:
  **medium**: the selected row is the TUI's focus.

**TC2: The help text is dim on dark terminals.**
- `help.go:32` renders the whole key list with `Dim` (colour 59,
  `styles.go:51-52`): 3.29:1 on black, 6.39:1 on white. The `Help` style
  (colour 246, `styles.go:47-49`; 6.92 on black) is defined and never used.
- The title (colour 39, `styles.go:20-23`) is 8.58 on black and 2.45 on white.
- Criterion: 1.4.3 by analogy, depending on the user's theme.
  Classification: **product**. Severity: **low**.

**Pass — meaning not by colour alone (1.4.1):** project status is printed as
text inside brackets, and only coloured on top (`app.go:159-166`,
`getStatusColor` `:242-254`).

**Pass — behaviour with colour off (read from code, not executed):** lipgloss
v1.1.0 takes its profile from termenv's `EnvColorProfile`
(`github.com/charmbracelet/lipgloss@v1.1.0/renderer.go:74`), which returns the
`Ascii` profile when `NO_COLOR` is set (`github.com/muesli/termenv@v0.16.0/termenv.go:68-69`,
`:99-102`). With colour off the status text and the "▶ " marker remain.

### 2.4 Screen-reader output, structure, motion

- **Not checkable here:** the TUI runs in the alternate screen
  (`tui.go:42`) and re-fetches and re-renders every second (`app.go:78-79`,
  `:262-265`). How Orca or VoiceOver reads that redraw needs a real terminal
  screen reader (see "After deploy").
- **Structure:** not applicable (no markup).
- **Motion:** there is no animation. The one-second refresh is
  auto-updating content with no pause control; 2.2.2 may apply by analogy.
  **no clause**, advisory. Severity: **low**.

## Checks run

Worktree HEAD `ebf32d3e`, clean tree before and after (the temporary probe
files were deleted; `git status` shows only ignored `ui/node_modules/`).

| Command | Exit | Summary output |
|---|---|---|
| `run_declared_check lint` (`go vet ./...`, then `cd ui && npm run lint` = `eslint src --max-warnings 0`) | 0 | "lint exited 0."; no findings |
| `run_declared_check go-test ./internal/tui/... -count=1` | 0 | `? github.com/openexec/openexec/internal/tui [no test files]` |
| **ui/** `node node_modules/eslint/bin/eslint.js src --max-warnings 0` | 0 | no output |
| **ui/** `node --max-old-space-size=8192 node_modules/vitest/vitest.mjs run --silent --reporter=dot --fileParallelism=false --maxWorkers=1 --minWorkers=1` (serial, as in step 4 and `make test`) | 0 | `Test Files 44 passed (44)`, `Tests 661 passed (661)`; "WebSocket server error: Port is already in use" printed once |
| `go test ./internal/tui/... -count=1` from `ui/` | 1 | `lstat ./internal/tui/: no such file or directory` — wrong working directory; re-run by absolute path below |
| `go test <worktree>/internal/tui/... -count=1` | 0 | `[no test files]` |
| `node ui/node_modules/@playwright/test/cli.js test --config ui/playwright.d4.config.ts --reporter=line` (the D4 Chromium journey from `make d4-test`, `Makefile:109-112`) | not captured (piped to `tail`) | `1 passed (10.7s)`. Shows Chromium runs in this worktree |
| Temporary Chromium probe, run 1 | not captured | 1 passed, 2 failed: a probe step clicked through the new-session overlay (the overlay ignored Escape, WK3), and the Usage mock returned `[]` |
| Temporary Chromium probe, run 2 (overlay closed by click; usage API mocked as 503) | not captured | `3 passed`; source of the Tab order, outlines, target sizes, structure, reflow and reduced-motion observations |
| Temporary Chromium probe, ARIA snapshot, run 1 | not captured | failed: the filter toggle was covered by the header (WF6) |
| Temporary Chromium probe, ARIA snapshot, run 2 (toggle opened by keyboard) | not captured | `1 passed`; source of WS1, WS3, WR2 names |
| `python3 -I <tmp>/contrast.py` (WCAG relative-luminance ratios; outside the repository, not committed) | 0 | the ratios quoted in 1.3 and 2.3 |

**The temporary probe.** A Playwright spec and config placed in `ui/e2e/` and
`ui/` for the run and deleted afterwards. It started only the Vite dev server
on 127.0.0.1:13001, stubbed `/ws`, and mocked every `/api` call (one session;
empty lists; usage 503). It pressed Tab 30 times and logged the focused
element's tag, name, computed outline and size; logged landmark, heading and
live-region counts per view; focused and hovered a session row; opened the
new-session modal and pressed Escape; measured all visible controls under
24px; and took ARIA snapshots. It used no axe package (none is installed, and
adding one is out of scope). It is not committed, so the examiner cannot
re-run it; its results are labelled **executed by hand** and none is claimed
as a passed check.

**What the checks show and do not show.**
- Lint and vitest pass, but neither contains an accessibility rule or
  assertion (A0.1), so they are not accessibility evidence.
- The D4 journey shows Chromium is available; it tests approvals, not
  accessibility.
- No daemon, model or real streaming ran. Every streamed-output statement
  (WR1) is read from code.

**Citations.** Every `path:line` was read with `grep -n`, `sed -n` or `cat -n`
in this worktree at `ebf32d3e` with a clean tree, which equals
`git show ebf32d3e:<path>`. The lipgloss and termenv lines are from the Go
module cache at the versions pinned in `go.mod` (`:11`, `:72`). The playbook
lines are from the local Agent Console checkout.

## Overlaps with earlier reports and repair tasks

| Earlier finding / task | Relation here |
|---|---|
| Step 4 U1/U2 (chat protocol mismatch, no streamed messages from the daemon) | WR1 is the missing announcement layer, independent of U2 |
| Step 4 U5 (unmounted `diff/`, `restart/` components) | WF3 and the unmounted rows in WC1/WC2 are on that dead surface; severity lowered accordingly |
| Step 4 TU2 (`?` does nothing) | Referenced in TK1, not repeated |
| Step 4 TU1 (log view has no project logs) | TK1's `g`/`G` defect is separate |
| Console tasks 684d and 61e; task 12f61 (selected-run behaviour); Goal 9921e5 / US-1000 (macOS fixture) | No finding here touches their scope or proposes their fixes |

## Limitations

- **No served, daemon-backed or streaming check.** The probe ran against
  mocked APIs on the Vite dev server, not the embedded build or a daemon.
  Chat streaming, approval flows with real data and the fork dialog (WF1)
  were not exercised.
- **No real screen reader.** NVDA, VoiceOver and Orca were not run, for the
  web UI or the TUI. WR1 and WR2 rest on source search and Chromium's ARIA
  snapshot, which is what a screen reader would be given, not what it says.
- **No axe or automated rule engine.** None is installed (A0.1); adding one is
  out of scope. Checks an engine would make broadly (for example every
  colour pair in every state) were done only for the pairs listed.
- **Contrast** used the hex values in source, not rendered pixels. Hover,
  disabled and translucent (`#xxxxxx20`) backgrounds were not computed.
- **Target size** was measured only on the Chat view's initial state; the
  spacing exception was not evaluated.
- **The TUI was not run.** Contrast assumes standard xterm palette values;
  real terminals may differ.
- **The probe is not reproducible by the examiner** because it is not
  committed. The source citations behind each executed-by-hand observation
  are given so they can be checked by reading.
- **Fixes are out of scope.** Every product finding is an engineering defect
  inside existing work and can be filed as a native task without an owner
  question.

## After deploy (supervisor measurements)

- Check real screen-reader output (NVDA, VoiceOver or Orca) on the served UI
  and in `openexec tui`.
- Confirm keyboard and focus behaviour on the served UI against a live daemon
  with real streaming, including the fork dialog (WF1) and announcements
  (WR1).

## Downstream

The playbook order (Console `internal/playbook/playbook.go:251-301`) is
interface (step 4, on `origin/main` as `0a6849bb`) → accessibility (this
report) → the consolidating report (`8a800117…`), which keeps its dependency on
this one.

Consolidated, re-checked at `131b9ce8`: [DESIGN_REVIEW_CONSOLIDATED_2026-10.md](DESIGN_REVIEW_CONSOLIDATED_2026-10.md).
