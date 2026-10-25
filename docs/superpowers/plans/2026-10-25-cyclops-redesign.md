# Cyclops compact dashboard redesign

Approved in chat: build system, AI, and mixed presets; use implementation judgment.
Keep the existing Go renderer, bitmap font, local provider data and USB transport.

Design: 240px circle, 6px rim inset, concentric thin metric rings, a 160px central
content area, large whole-pixel numbers and short labels. Palette: ink #080D14,
white #E7EFF0, cyan #62DCE5 CPU/OpenAI, amber #FFAC62 GPU/Claude, violet #C4A0FF
memory. Color identifies readings; labels and values carry their meaning too.
Graphs use the last two minutes with explicit gaps for missing samples.

1. Add reusable ring-only, compact number and sparkline components. Preserve
   the existing radial/centered widgets and saved layouts. Test transparent ring
   interiors, missing samples, bounded geometry and persistence.
2. Add AI provider orbit and status-strip detail modes rendered at native size.
   Reuse the rectangular provider presentation for status, quotas and tokens.
   Only change layout and shorten labels; no new observers or hook setup.
   Test status parity, quota fallback, sign-in and transparent circular rendering.
3. Recompose thermal, graphics, memory, dual-temperature and CPU presets. Add
   system overview, OpenAI orbit, Claude orbit, dual AI status and mixed console.
4. Expose components and AI detail modes in Studio; match canvas preview to the
   renderer. Default newly added circular AI widgets to orbit. Render actual AI
   thumbnails instead of scaling desktop cards to unreadable sizes.
5. Inspect a native-size contact sheet, build/vet/test, review, install with a
   backup, and apply a new mixed preset to the Cyclops. Keep the saved original
   Thermal dial and every other display binding intact.

Review focus: all AI layouts must share the existing status and usage decisions; graph gaps
must not become continuous lines; tiny widgets must not panic; text and quotas
must fit the circle; new preset selection must not overwrite saved layouts.
