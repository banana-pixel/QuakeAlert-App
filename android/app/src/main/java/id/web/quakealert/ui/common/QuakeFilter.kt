package id.web.quakealert.ui.common

/**
 * The two filter modes shown in the shared [QuakeFilterRow] across the History
 * (Figma node 1:711) and Sensors (Figma node 1:1105) screens. Both designs use
 * an identical "All" / "Near - {radius}km" pill pair, so a single enum is shared
 * to avoid divergent per-screen duplicates.
 */
enum class QuakeFilter { ALL, NEAR }
