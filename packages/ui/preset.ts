export default {
  theme: {
    extend: {
      colors: {
        ds: {
          paper: "var(--ds-paper)",
          panel: "var(--ds-panel)",
          panelStrong: "var(--ds-panel-strong)",
          line: "var(--ds-line)",
          ink: "var(--ds-ink)",
          muted: "var(--ds-muted)",
          accent: "var(--ds-accent)",
          accentSoft: "var(--ds-accent-soft)",
          accentHover: "var(--ds-accent-hover)",
          positive: "var(--ds-positive)",
          warning: "var(--ds-warning)",
          danger: "var(--ds-danger)"
        }
      },
      boxShadow: {
        panel: "var(--ds-shadow-panel)",
        focus: "0 0 0 4px color-mix(in srgb, var(--ds-accent) 18%, transparent)"
      },
      borderRadius: {
        shell: "var(--ds-radius-shell)",
        panel: "var(--ds-radius-panel)",
        chip: "999px"
      },
      fontFamily: {
        display: "var(--ds-font-display)",
        body: "var(--ds-font-body)"
      }
    }
  }
}
