# Display Studio and AI widget redesign

The configurator opens each display's actual assigned output. CPU, GPU and
memory fans no longer all fall back to the first fan's template. The editor's
**On display** view shows the renderer assigned to the selected serial;
**Edit layout** opens that screen's saved composition or an editable starting
layout matching its hardware/provider role. Late responses from another display
cannot replace the current preview.

![Each display has its own assigned output](screenshots/display-overview.png)

Display cards, navigation, labels and controls use a larger type scale. The
workspace keeps the selected target, layout state and Save & apply visible.
Offline format controls are hidden for selected physical displays. The widget
library has separate ChatGPT / Codex and Claude entries. Selecting either widget
shows its provider connection inside the inspector; there is no separate AI
connection page. Existing sign-ins are detected, and Sign in or Reconnect appears
only when required. Monitoring and optional activity setup remain beside that
widget under connection options. Switching providers clears sign-in guidance and
ignores delayed setup responses from the previous provider. Shared layouts show a warning
that saving updates all displays using that layout; Duplicate makes a separate
composition.

![AI setup belongs to the selected widget](screenshots/ai-widgets.png)

Screenshots use an isolated four-display demo with sample readings and quotas.

Claude uses the square-eyed pixel crab from the user's reference and
[motion reference](https://tenor.com/view/claude-claude-code-claw%27d-crab-laptop-gif-14833619646318452398).
ChatGPT uses the blossom from the [official icon reference](https://help.openai.com/en/articles/7905742-what-does-the-official-chatgpt-ios-app-icon-look-like),
rendered in monochrome on the display. Mascot poses are computed per frame in
Go; they do not play a GIF or MP4. Running, waiting and connected idle states
have distinct movement. Fresh local activity can animate independently of
account sign-in. Stale activity cannot animate, and stale quotas retain their
freshness warning. Animation off remains stable; the browser also honors reduced
motion.

New fan widgets occupy 608 × 164 pixels, with 42-pixel quota values, 28-pixel
token values and 14-pixel supporting labels. Large token counts use K/M/B
abbreviations. Existing smaller widgets remain compatible, with a responsive
compact arrangement. Expanded 300-pixel widgets keep session title, usage and
context visible. Missing values stay unavailable; local/partial observations are
identified separately from account quota.

Verification: Go tests and vet, Node editor/browser metadata tests, desktop and
narrow browser inspection, all three fan roles, pixel comparison against the
assigned memory renderer, and Save & apply isolation with a temporary four-screen
test service. Test fixtures never send frames to physical displays.
Overview-card responses are also checked against the current assignment and
credential epoch, with stale cached images discarded after reassignment.
