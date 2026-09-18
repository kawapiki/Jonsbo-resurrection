# Configurator usability redesign

The first editor exposed every control at once, placed themes below the canvas, and required a separate save before applying. The redesign puts display selection first, visual libraries on the left, the composition in the middle, and settings for the selected widget on the right.

## Research

- [Corsair ELITE LCD setup](https://help.corsair.com/hc/en-us/articles/4412131516813-ELITE-LCD-How-to-set-up-your-ELITE-LCD-in-iCUE-4-or-newer): choose the device, screen type, then customize its settings.
- [Elgato background library](https://www.elgato.com/ca/en/explorer/products/stream-deck/how-to-add-a-background-and-screensaver/): browse visual backgrounds and import personal media.
- [Figma properties panel](https://help.figma.com/hc/en-us/articles/360039832014-Design-prototype-and-explore-layer-properties-in-the-right-sidebar): change controls according to the selected canvas element.

These patterns informed the workflow; no external UI assets or dependencies were copied.

## Design decisions

Use cloud #EDF1F6 for the workspace, white #FFFFFF for tools, ink #24324A for text, slate #64748A for secondary text, and cobalt #345AD8 for actions. Segoe UI Variable keeps Windows controls familiar. The dark physical-screen frame is the focal point; the surrounding controls stay light and quiet.

Keep the main action in the header. Separate Themes, Widgets and Background library views. Hide exact coordinates and chart ranges in expandable sections. Fit the canvas to available space and let tool panes scroll independently on desktop. On narrow windows, stack panels without hiding functionality.

Save & apply saves the captured draft, then verifies that the revision and selected display are unchanged before assigning it. Failed or superseded saves cannot assign a layout. Duplicate creates an independent ID so a user can explore without modifying a layout already used by another display.

The hardware-free local preview remains explicitly labeled. Missing readings remain unavailable; theme thumbnails do not invent sensor values.
