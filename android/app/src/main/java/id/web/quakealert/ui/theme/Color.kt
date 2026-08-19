package id.web.quakealert.ui.theme

import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color

// ============================================================
// Flat Minimalist Dark palette — QuakeAlert Onboarding (Figma)
// ============================================================

// Background gradient stops
// linear-gradient(180deg, rgba(0, 24, 42, 1) 0%, rgba(0, 0, 0, 1) 100%)
val BackgroundGradientTop = Color(0xFF00182A)
val BackgroundGradientBottom = Color(0xFF000000)

// Primary accent — button fill rgba(0, 85, 125, 0.5)
val AccentBlue = Color(0xFF00557D)
val AccentBlueTranslucent = Color(0x8000557D) // 50% alpha

// Neutrals / text
val TextPrimary = Color(0xFFFFFFFF)
val TextSecondary = Color(0x85FFFFFF) // white 52% (0.52 * 255 ≈ 0x85)

// Surfaces & borders
val SurfaceDark = Color(0xFF00182A)
val BorderLight = Color(0x4DFFFFFF) // white 30%
val OverlayLight = Color(0x0DFFFFFF) // white 5% (progress track "Black" overlay)

// Permission card (Onboarding Page 3)
val CardDark = Color(0xFF222222) // card fill #222222
val BorderFaint = Color(0x1AFFFFFF) // white 10%
val SuccessGreenTranslucent = Color(0x78163B00) // rgba(22, 59, 0, 0.47)
val SuccessGreen = Color(0xFF4CAF50) // granted badge accent

// Clickable text links (Onboarding Page 7)
val TextLink = Color(0xFF4FC3D9) // teal/cyan link accent

// ============================================================
// History screen palette — QuakeAlert (Figma node 1:701)
// ============================================================

// Page & card surfaces
val HistoryBackground = Color(0xFF000000)       // frame fill #000000
val CardSurface = Color(0xFF222222)             // history card fill #222222
val CardBorder = Color(0x1AFFFFFF)              // white 10% card stroke

/**
 * Shared stadium-capsule fill for the QuakePill used by both the History card's
 * "km Away" badge and the Sensor card's telemetry pills (#373737). Single source
 * of truth so the two cards' capsules are byte-identical.
 */
val PillFill = Color(0xFF373737)                // shared capsule fill #373737


// Top-bar "Healthy" status badge
val HealthyBadgeFill = Color(0xFF0C3600)        // green badge fill #0C3600

// Filter pills
val FilterActiveFill = Color(0xFF003346)        // "All" active fill #003346
val FilterInactiveFill = Color(0xFF222222)      // inactive fill #222222

// MMI (intensity) badge — moderate (orange) vs severe (red)
val MmiOrange = Color(0xFFF39F1D)               // intensity accent / text
val MmiOrangeContainer = Color(0xFF462C08)      // orange badge fill
val MmiRed = Color(0xFFF31D1D)                  // intensity accent / text
val MmiRedContainer = Color(0xFF460808)         // red badge fill

// Card sub-elements
val MapPlaceholder = Color(0xFFD9D9D9)          // map thumbnail placeholder
val DistanceBadgeFill = Color(0xFF373737)       // "X km Away" pill fill
val ShareButtonFill = Color(0xFF24505D)         // share icon button fill

// Bottom navigation
val NavBarFill = Color(0xFF232323)              // nav container fill #232323
val NavBarBorder = Color(0x26FFFFFF)            // white 15% nav stroke
val NavActiveFill = Color(0x6300677E)           // active tab pill rgba(0,103,126,0.39)
val NavActiveText = Color(0xFF5C98AB)           // active tab label/icon tint
val NavLabel = Color(0x99FFFFFF)                // inactive tab label (dimmed white)

// ============================================================
// Sensors screen palette — QuakeAlert (Figma node 1:1081)
// ============================================================

// Sensor "chip" badge (MPU-6050) — leading icon container
val SensorChipFill = Color(0xFF7EB1C7)          // cpu-chip badge fill #7EB1C7
val SensorChipBorder = Color(0xFF214F68)        // cpu-chip badge stroke #214F68

// Station status chips
val StatusOnlineFill = Color(0xFF0C3600)        // "Online" chip fill #0C3600
val StatusOfflineFill = Color(0xFF360000)       // "Offline" chip fill #360000
val StatusOnlineDot = Color(0xFF4CAF50)         // solid green dot indicator (online)
val StatusOfflineDot = Color(0xFFF31D1D)        // solid red dot indicator (offline)


// Telemetry pills (Last Ping / RSSI / Latency)
val TelemetryPillFill = Color(0xFF373737)       // telemetry pill fill #373737

// Map preview card
val MapLocationPillFill = Color(0x78000000)     // location pill rgba(0,0,0,0.47)
val MapRangeBadgeFill = Color(0xFF1B536A)       // "Range : ..." badge fill #1B536A
val MapSettingsShortcutFill = Color(0xFFD9D9D9) // settings shortcut circle #D9D9D9
val MapSettingsShortcutBorder = Color(0x1A000000) // black 10% stroke

// Reactive coverage geofence circle (Settings "Location & Coverage" map)
val GeofenceFill = Color(0x331B536A)            // translucent teal coverage fill
val GeofenceStroke = Color(0xFF1B536A)          // solid teal coverage ring #1B536A


// Highlighted station-id suffix ("NODE-...")
val SensorNodeIdText = Color(0xFF7EB1C7)        // node-id accent #7EB1C7 (ts2 #7EB1C7)

// ============================================================
// Settings screen palette — QuakeAlert (Figma node 1:845)
// ============================================================

// Section header pill ("Location & Coverage", "About", ...)
val SectionHeaderPillFill = Color(0xFF2D2D2D)   // header pill fill #2D2D2D

// Setting row cards reuse CardSurface (#222222); border uses white 30%.
val SettingCardBorder = Color(0x4DFFFFFF)       // white 30% setting-card stroke

// Coverage / Language segmented toggle pills. Active option uses the solid cyan
// FilterActiveFill (shared with the History filter row) per the Figma spec.
val SegmentActiveFill = FilterActiveFill        // active segment cyan #003346
val SegmentInactiveFill = Color(0x47161616)     // inactive segment rgba(22,22,22,0.28)

// Custom M3-style switch
val SwitchTrackActive = Color(0xFF333333)       // active track fill #333333
val SwitchTrackInactive = Color(0xFF2A2A2A)     // inactive track fill
val SwitchThumbActive = Color(0xFFFFFFFF)       // active thumb (white)
val SwitchThumbInactive = Color(0x99FFFFFF)     // inactive thumb (dimmed white)

// "More About Us" call-to-action button
val AboutButtonFill = Color(0xFF6A411B)         // about CTA fill #6A411B (caramel)

// About card gradient (linear 90deg khaki → green, both 28% alpha)
val AboutGradientStart = Color(0x47807A41)      // rgba(128,122,65,0.28)
val AboutGradientEnd = Color(0x47087900)        // rgba(8,121,0,0.28)
val AboutCardGradient = Brush.horizontalGradient(
    colors = listOf(AboutGradientStart, AboutGradientEnd)
)
/** Soft green stroke around the About card (Figma node 1:918). */
val AboutCardBorder = Color(0x66087900)         // green 40% about-card stroke

// ============================================================
// About modal / overlay palette — QuakeAlert (Figma node 4:654)
// ============================================================

// Modal card fill (node 4:668):
// linear-gradient(180deg, rgba(1,49,54,1) 0%, rgba(0,18,27,1) 100%)
val AboutModalGradientTop = Color(0xFF013136)   // rgba(1,49,54,1)
val AboutModalGradientBottom = Color(0xFF00121B) // rgba(0,18,27,1)
val AboutModalGradient = Brush.verticalGradient(
    colors = listOf(AboutModalGradientTop, AboutModalGradientBottom)
)

/**
 * Circular close (X) button container shared by every overlay header — the About
 * modal (node 4:761) and the Earthquake Details modal (node 123:1006) both ship
 * the identical rgba(217,217,217,0.35) disc, so it is named for the pattern
 * rather than for either screen.
 */
val ModalCloseFill = Color(0x59D9D9D9)

// Action button fills (nodes 4:677 / 4:681 / 4:686). All three are the same
// 31%-alpha wash over the modal gradient, differing only in hue.
val AboutActionGithubFill = Color(0x4F00B9E3)   // rgba(0,185,227,0.31) teal/cyan
val AboutActionEmailFill = Color(0x4FE3D400)    // rgba(227,212,0,0.31) olive/gold
val AboutActionDonateFill = Color(0x4FAB3600)   // rgba(171,54,0,0.31) warm bronze

// Concentric logo badge (node 4:670). Figma ships this as a raster logo fill; it
// is rebuilt from Compose primitives as three concentric discs stepping up in
// alpha from the shared #00B9E3 accent (same hue as the GitHub action) so the
// badge reads as a soft glow around the seismograph glyph.
val AboutLogoHaloFill = Color(0x1400B9E3)       // cyan 8% — outermost halo
val AboutLogoRingFill = Color(0x2E00B9E3)       // cyan 18% — middle ring
val AboutLogoCoreFill = Color(0x4F00B9E3)       // cyan 31% — core disc

// ============================================================
// Earthquake Details overlay palette — QuakeAlert (node 123:1002)
// ============================================================

// Modal card fill (node 123:1002):
// linear-gradient(180deg, rgba(70,44,8,1) 0%, rgba(34,34,34,1) 100%). The bronze
// top stop is the same #462C08 the moderate MMI badge uses, and the bottom stop
// lands exactly on CardSurface, so the overlay reads as a History card lifted
// off the list and warmed at the top.
val EventDetailGradientTop = Color(0xFF462C08)    // rgba(70,44,8,1) bronze
val EventDetailGradientBottom = Color(0xFF222222) // rgba(34,34,34,1) — CardSurface
val EventDetailModalGradient = Brush.verticalGradient(
    colors = listOf(EventDetailGradientTop, EventDetailGradientBottom)
)

/**
 * Severe-variant top stop (node 124:1192): rgba(70,8,8,1) — the same #460808 the
 * severe MMI badge container uses, so the alert detail card is "the severe badge
 * warmed at the top" just as the moderate card is its bronze badge warmed at the
 * top. Bottom stop stays on [EventDetailGradientBottom].
 */
val EventDetailGradientTopSevere = Color(0xFF460808) // rgba(70,8,8,1) dark red
val EventDetailModalGradientSevere = Brush.verticalGradient(
    colors = listOf(EventDetailGradientTopSevere, EventDetailGradientBottom)
)

/**
 * Inset panel fill shared by the three seismic metric cells (node 124:1115) and
 * the spatial info card (node 124:1147) — rgba(0,0,0,0.31). A black wash rather
 * than an opaque surface, so the card's gradient still shows through and the
 * panels stay visually recessed at both ends of the gradient.
 */
val MetricPanelFill = Color(0x4F000000)

/** Hairline rule between the two spatial info rows (node 124:1151) — #5D5D5D. */
val EventDetailDividerColor = Color(0xFF5D5D5D)

/**
 * Bottom "Share" button fill (node 124:1085) — rgba(171,54,0,0.31). The same
 * warm-bronze 31% wash the About overlay's Donate action carries; kept as its own
 * token because the two buttons are unrelated actions that merely share a hue.
 */
val EventDetailShareFill = Color(0x4FAB3600)

/**
 * Pulse rings drawn over the detail map thumbnail. Figma ships a rendered map
 * raster here (node 123:1028); pending the map SDK the epicentre is expressed as
 * concentric rings over the shared [MapPlaceholder] surface, tinted by the event's
 * MMI accent at these alphas so the focus reads without hiding the map beneath.
 */
val EventDetailPulseOuterAlpha = 0.12f
val EventDetailPulseMidAlpha = 0.20f
val EventDetailPulseInnerAlpha = 0.34f


// Last-sync / info pill on setting cards
val InfoPillFill = Color(0x78000000)            // rgba(0,0,0,0.47) info pill fill

// ============================================================
// Chat screen palette — QuakeAlert (Figma node 1:925)
// ============================================================

// Channel/Network card gradient (linear 90deg, teal-blue → olive-green)
// linear-gradient(90deg, rgba(0,52,86,1) 0%, rgba(32,52,0,1) 100%)
val ChatChannelGradientStart = Color(0xFF003456)  // rgba(0,52,86,1)
val ChatChannelGradientEnd = Color(0xFF203400)    // rgba(32,52,0,1)
val ChatChannelCardGradient = Brush.horizontalGradient(
    colors = listOf(ChatChannelGradientStart, ChatChannelGradientEnd)
)

// Incoming message bubble reuses CardSurface (#222222) + CardBorder (white 10%).
val ChatIncomingFill = CardSurface                // #222222
// Outgoing message bubble — dark teal/cyan fill #032B39 with a cyan accent stroke.
val ChatOutgoingFill = Color(0xFF032B39)          // outgoing bubble fill #032B39
val ChatOutgoingBorder = Color(0x4D0998CC)        // cyan 30% outgoing bubble stroke

// Chat input field & send button
val ChatInputFill = Color(0x47FFFFFF)             // input container rgba(255,255,255,0.28)
val ChatSendButtonFill = Color(0x470998CC)        // send button rgba(9,152,204,0.28)



// "Sync Location Now" refresh action — dark rounded container behind the icon
val SyncButtonFill = Color(0xFF2A2A2A)          // refresh action container fill #2A2A2A







// ============================================================
// Warning screen palette — QuakeAlert (Figma node 1:1024)
// ============================================================

// Alert banner card — deep crimson vertical gradient
// linear-gradient(180deg, rgba(175,0,0,1) 0%, rgba(76,2,2,1) 100%)
val AlertBannerGradientTop = Color(0xFFAF0000)     // rgba(175,0,0,1)
val AlertBannerGradientBottom = Color(0xFF4C0202)  // rgba(76,2,2,1)
val AlertBannerGradient = Brush.verticalGradient(
    colors = listOf(AlertBannerGradientTop, AlertBannerGradientBottom)
)

/** "SEE DETAILS" capsule stroke — translucent white 30% (Figma fill_a7745cf9). */
val AlertActionBorder = Color(0x4DFFFFFF)          // white 30%

/**
 * No-active-quake banner card — amber/orange vertical gradient (Figma 124:1426).
 * The resting state's banner: warmer than the crimson alert gradient so the two
 * states read at a glance, but still unmistakably a warning tone.
 * linear-gradient(180deg, rgba(175,97,0,1) 0%, rgba(76,44,2,1) 100%)
 */
val PossibilityBannerGradientTop = Color(0xFFAF6100)   // rgba(175,97,0,1)
val PossibilityBannerGradientBottom = Color(0xFF4C2C02) // rgba(76,44,2,1)
val PossibilityBannerGradient = Brush.verticalGradient(
    colors = listOf(PossibilityBannerGradientTop, PossibilityBannerGradientBottom)
)

/**
 * Earthquake Possibility overlay card (Figma 124:1605) — a near-flat dark
 * gradient so the card reads as a lifted surface rather than an alert: the top
 * stop is the shared NavBarFill and the bottom stop lands exactly on CardSurface,
 * so the card sits between the nav bar and the cards behind it on the value axis.
 */
val PossibilityModalGradientTop = Color(0xFF232323)    // rgba(35,35,35,1)
val PossibilityModalGradientBottom = Color(0xFF222222) // rgba(34,34,34,1) — CardSurface
val PossibilityModalGradient = Brush.verticalGradient(
    colors = listOf(PossibilityModalGradientTop, PossibilityModalGradientBottom)
)

/** Short subtle drag-handle divider between the banner and the tips (Figma 1:1037). */
val WarningDividerColor = Color(0x66FFFFFF)        // white 40%

/** Preparedness tip circle icon — transparent fill + thin white stroke. */
val PrepIconBorder = Color(0x4DFFFFFF)             // white 30% icon-circle stroke

/** Emergency bottom CTA — dark wine/crimson container rgba(179,54,54,0.31). */
val EmergencyCtaFill = Color(0x4FB33636)           // rgba(179,54,54,0.31)
val EmergencyCtaBorder = Color(0x4DFFFFFF)         // white 30% CTA stroke


/**
 * Vertical background gradient used across onboarding screens,
 * mirroring the Figma frame fill.
 */
val OnboardingBackgroundBrush = Brush.verticalGradient(
    colors = listOf(BackgroundGradientTop, BackgroundGradientBottom)
)

