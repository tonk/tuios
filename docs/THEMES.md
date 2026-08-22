# Themes

TUIOS ships a large set of built-in color themes and can load custom ones from
JSON files in your config directory. A theme supplies the 16 ANSI colors plus
foreground, background and cursor; TUIOS derives its own UI colors (borders,
overlays, the dockbar) from them.

## Table of Contents

- [Selecting a Theme](#selecting-a-theme)
- [Custom Themes](#custom-themes)
- [Theme File Format](#theme-file-format)
- [Defaults for Omitted Colors](#defaults-for-omitted-colors)
- [Per-Element Overrides](#per-element-overrides)
- [Limitations](#limitations)

## Selecting a Theme

By config file:

```toml
[appearance]
theme = "dracula"
```

By command line, which takes precedence over the config file:

```bash
tuios --theme dracula
tuios --list-themes                  # every registered theme ID, custom ones included
tuios --preview-theme dracula        # print the theme's 16 ANSI colors
tuios --theme $(tuios --list-themes | fzf --preview 'tuios --preview-theme {}')
```

In the running app, the command palette (`Ctrl+P`) has a **Theme Picker** entry,
and the settings page (`Ctrl+B` `,`) has a Theme row that opens the same picker.
The picker is searchable and shows a color swatch for each theme; cancelling
restores the theme that was active when you opened it.

Leaving the theme unset disables theming entirely and TUIOS uses your terminal's
own colors. An unknown theme name logs a warning and leaves the colors as they
were, rather than failing to start.

## Custom Themes

Custom themes are `.json` or `.toml` files in the themes directory:

```
~/.config/tuios/themes/
```

More precisely `$XDG_CONFIG_HOME/tuios/themes/`, following the same XDG rules as
the config file. The directory is created for you.

Every `*.json` and `*.toml` file directly in that directory is loaded at
startup and registered alongside the built-in themes, which means a custom
theme can be selected by `theme = "..."`, by `--theme`, and from the picker
exactly like a built-in one. Subdirectories are not scanned. A file that fails
to parse is skipped with a warning in the log and does not prevent the other
themes, or the app, from loading.

`.toml` is the newer of the two formats and the only one that supports
comments (`#`). Prefer it for anything you plan to hand-edit; `.json` still
works exactly as before for existing theme files.

Themes are read once, at startup. Adding or editing a theme file requires a
restart.

## Theme File Format

The file is a JSON object. Colors may be written either as a hex string or as an
RGBA object:

```json
{
  "id": "my-theme",
  "display_name": "My Theme",

  "fg": "#e5e5e5",
  "bg": "#101014",
  "cursor": "#e5e5e5",

  "black":   "#1b1b23",
  "red":     "#e06c75",
  "green":   "#98c379",
  "yellow":  "#e5c07b",
  "blue":    "#61afef",
  "purple":  "#c678dd",
  "cyan":    "#56b6c2",
  "white":   "#abb2bf",

  "bright_black":  "#4b5263",
  "bright_red":    "#ef7a83",
  "bright_green":  "#a9d18a",
  "bright_yellow": "#f0cc8c",
  "bright_blue":   "#72bcff",
  "bright_purple": "#d788ee",
  "bright_cyan":   "#67c5d3",
  "bright_white":  "#ffffff"
}
```

The RGBA form for any color field is `{"r": 255, "g": 0, "b": 0, "a": 255}`.

The same theme as TOML - identical fields, hex strings only (no RGBA-object
form in this format), and comments:

```toml
id = "my-theme"
display_name = "My Theme"

fg = "#e5e5e5"
bg = "#101014"
cursor = "#e5e5e5"  # defaults to fg if omitted

black   = "#1b1b23"
red     = "#e06c75"
green   = "#98c379"
yellow  = "#e5c07b"
blue    = "#61afef"
purple  = "#c678dd"
cyan    = "#56b6c2"
white   = "#abb2bf"

bright_black  = "#4b5263"
bright_red    = "#ef7a83"
bright_green  = "#a9d18a"
bright_yellow = "#f0cc8c"
bright_blue   = "#72bcff"
bright_purple = "#d788ee"
bright_cyan   = "#67c5d3"
bright_white  = "#ffffff"
```

Two fields control identity:

- `id` is the name you select the theme by. If it is omitted, it is derived from
  the filename: `~/.config/tuios/themes/My-Theme.json` becomes `my-theme`
  (lowercased, extension stripped).
- `display_name` is what the picker shows. If omitted it falls back to the `id`.

Note the color names: TUIOS uses `purple`, not `magenta`.

## Defaults for Omitted Colors

Every color field is optional. A field you leave out is filled in rather than
left unset, so a partial theme is valid:

| Field | Fallback |
|---|---|
| `fg` | `#e5e5e5` |
| `bg` | `#000000` |
| `cursor` | the resolved `fg` |
| `black`, `red`, `green`, `yellow`, `blue`, `purple`, `cyan`, `white` | the xterm defaults (`#000000`, `#cd0000`, `#00cd00`, `#cdcd00`, `#0000ee`, `#cd00cd`, `#00cdcd`, `#e5e5e5`) |
| any `bright_*` | its non-bright counterpart |

This means a theme that defines only the eight normal colors will render with
bright text indistinguishable from normal text, which is usually not what you
want. Define the bright variants explicitly.

## Per-Element Overrides

The 16 ANSI colors plus `fg`/`bg`/`cursor` are also the palette guest programs
render with inside a pane, so TUIOS derives its own chrome (borders, dock
indicators, the cursor, copy-mode highlighting, notification colors, and the
overlay accent) from that same handful of fields - e.g. the focused-terminal
border is always `bright_green`, the focused-window border is always
`bright_cyan`. See [CONFIGURATION.md](CONFIGURATION.md) if you only want to
retint pane borders; the `border_focused_color`/`border_unfocused_color`
settings there cover that without touching the theme file.

An optional `ui` object (JSON) or `[ui]` table (TOML) assigns a color to one
of those elements directly, instead of accepting whatever the derivation above
picks. Every key is optional and a hex string; anything left out keeps using
the derived color as before:

```toml
[ui]
border_focused_terminal = "#a6e3a1"  # instead of bright_green
border_focused_window   = "#89b4fa"  # instead of bright_cyan
border_unfocused        = "#585b70"  # instead of red
border_multifocused     = "#f9e2af"  # panes selected together for a broadcast action; instead of a fixed ANSI yellow
dock_window             = "#89b4fa"
dock_terminal           = "#a6e3a1"
dock_copy               = "#f9e2af"
dock_highlight          = "#a6e3a1"
dock_bg                 = "#11111b"  # the dock/statusbar row's own background - see below
dock_trail_fg           = "#a6adc8"  # the "<workspace>:<windows>" readout and its badges; instead of the derived FgMute
dock_indicator_active_fg   = "#a6e3a1"  # mode-indicator glyphs (mouse/tiling/focus-follows-mouse) while their mode is on; instead of the derived Success
dock_indicator_inactive_fg = "#585b70"  # the same glyphs while their mode is off; instead of the derived FgMute
workspace_pill_active_bg   = "#89b4fa"  # the current-workspace tab's fill; instead of the neutral chrome Panel step
workspace_pill_active_fg   = "#11111b"  # its label ink; instead of the derived accent
workspace_pill_inactive_bg = "#11111b"  # every other tab's fill; instead of the same neutral Panel step
workspace_pill_inactive_fg = "#a6adc8"  # its label ink; instead of the derived FgDim
terminal_cursor_fg      = "#f5e0dc"
terminal_cursor_bg      = "#000000"
button_fg               = "#000000"
copy_cursor_bg          = "#94e2d5"
copy_cursor_fg          = "#000000"
copy_visual_selection_bg = "#cba6f7"
copy_visual_selection_fg = "#ffffff"
copy_search_current_bg = "#f38ba8"
copy_search_current_fg = "#000000"
copy_search_other_bg   = "#f9e2af"
copy_search_other_fg   = "#000000"
copy_search_bar_bg     = "#f9e2af"
copy_search_bar_fg     = "#000000"
notification_error      = "#f38ba8"
notification_warning    = "#f9e2af"
notification_success    = "#a6e3a1"
notification_info       = "#89b4fa"
notification_bg         = "#1e1e2e"
notification_fg         = "#d4d4d4"
accent        = "#89b4fa"  # overlay chrome: dock highlights, active tabs, badges
accent_bright = "#94e2d5"
selected      = "#89b4fa"
warn          = "#f38ba8"
success       = "#a6e3a1"
info          = "#89b4fa"
warning       = "#f9e2af"
```

Only custom (file-based) themes can carry a `ui`/`[ui]` section - a built-in
theme comes from the vendored color-scheme library as plain color data, with
no room for one. Most of the neutral overlay chrome (panel backgrounds, plain
text, help/CLI-table colors) stays fixed regardless of the theme or this
section; `accent`/`accent_bright`/`selected`/`warn`/`success`/`info`/`warning`
are the overlay tokens that do follow the theme, and the only ones this
section can retint.

**`dock_bg` is different from the rest of this table**: every other key
*replaces* a color TUIOS would otherwise derive; `dock_bg` *adds* one where
there is normally none at all. The dock/statusbar row - the mode pill, the
workspace tabs, the window list, the system stats - paints no background of
its own by default; its bare cells (the gaps between pills, the padding
around the right-hand block) show the real terminal's own color, same as any
other unstyled corner of the screen. Setting `dock_bg` is the equivalent of
tmux's `status-style bg=...`: it gives that entire row a solid background, and
every foreground color that reads against it (arrows, session controls, pill
caps) is measured for contrast against it rather than against the neutral
`pal.Canvas` guess used when it's unset. Elements that already carry their own
fill - a workspace pill, a minimized-window pill, the notification message
body - keep their own color on top of it, the same way tmux's
`window-status-current-style` overrides `status-style` for just the active
tab. The separator hairline directly above the row is unaffected either way.

## Limitations

- **Not watched, but reloadable.** Nothing watches the themes directory the
  way `config.toml` itself is watched, so editing a theme file isn't picked up
  the moment you save it. It doesn't need a full restart either, though:
  `Ctrl+B D r` (`debug_prefix_reload_theme`) re-scans the directory and
  applies changes to the active theme immediately, and saving `config.toml`
  (or the command palette's "Reload Config") does the same as a side effect
  of its own reload. Switching between already-registered themes from the
  picker or the settings page always applied immediately, reload or not.
- **Flat directory.** Only `*.json` and `*.toml` files directly under the
  themes directory are loaded; subdirectories are ignored.
- **No validation beyond parsing.** A syntactically valid file with meaningless
  colors loads happily. Use `tuios --preview-theme <id>` to check the result.
- **Border color overrides can come from two places.** `border_focused_color`/
  `border_unfocused_color` in `[appearance]` (config.toml) and a theme's own
  `border_focused_window`/`border_focused_terminal`/`border_unfocused` (the
  theme file's `ui` section) both exist; the `[appearance]` setting wins when
  both are set, since it's the more explicit, most-recently-set choice.
- **Some overlays are not themed.** The which-key popup, in particular, draws
  with fixed colors regardless of the active theme or its `ui` section.

## Related Documentation

- [CONFIGURATION.md](CONFIGURATION.md) - the config file and every other option
- [CLI_REFERENCE.md](CLI_REFERENCE.md) - `--theme`, `--list-themes`, `--preview-theme`
