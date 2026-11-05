import type { Mermaid, MermaidConfig } from 'mermaid'

// The diagrams in the Facets palette (style.css): ink and surfaces, lavender lines, violet
// borders, cyan for what stands aside (notes, second-level shapes).
const ink = '#0a0b16'
const surface = '#121430'
const raised = '#191c3e'
const highest = '#22264f'
const text = '#eef0fa'
const muted = '#9a9dbc'
const lavender = '#b2b8ff'
const violet = '#6d4aff'
const cyan = '#22d3ee'

const config: MermaidConfig = {
  startOnLoad: false,
  securityLevel: 'strict',
  theme: 'base',
  fontFamily: "'Geist', ui-sans-serif, system-ui, sans-serif",
  // Sequence diagrams keep their natural size, the box around them scrolling sideways
  // (Mermaid.vue); long messages wrap so that the actors stay close.
  sequence: {
    useMaxWidth: false,
    wrap: true,
    width: 110,
    actorMargin: 30,
    messageFontSize: 15,
    noteFontSize: 15,
    actorFontSize: 15,
  },
  themeVariables: {
    darkMode: true,
    background: ink,
    fontFamily: "'Geist', ui-sans-serif, system-ui, sans-serif",
    fontSize: '15px',
    textColor: text,
    primaryColor: raised,
    primaryTextColor: text,
    primaryBorderColor: violet,
    secondaryColor: surface,
    secondaryTextColor: text,
    secondaryBorderColor: cyan,
    tertiaryColor: surface,
    tertiaryTextColor: text,
    tertiaryBorderColor: lavender,
    mainBkg: raised,
    nodeBorder: violet,
    nodeTextColor: text,
    lineColor: lavender,
    clusterBkg: surface,
    clusterBorder: highest,
    titleColor: text,
    edgeLabelBackground: surface,
    // Sequence diagrams.
    actorBkg: raised,
    actorBorder: violet,
    actorTextColor: text,
    actorLineColor: muted,
    signalColor: lavender,
    signalTextColor: text,
    labelBoxBkgColor: surface,
    labelBoxBorderColor: cyan,
    labelTextColor: text,
    loopTextColor: text,
    noteBkgColor: surface,
    noteBorderColor: cyan,
    noteTextColor: text,
    activationBkgColor: highest,
    activationBorderColor: lavender,
    sequenceNumberColor: ink,
  },
}

let mermaid: Promise<Mermaid> | undefined

/** Draws a diagram as SVG, loading Mermaid on first use. */
export async function renderDiagram(id: string, source: string): Promise<string> {
  mermaid ??= import('mermaid').then(({ default: loaded }) => {
    loaded.initialize(config)
    return loaded
  })
  return (await (await mermaid).render(id, source)).svg
}
