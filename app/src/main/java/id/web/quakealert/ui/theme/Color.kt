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


// Last-sync / info pill on setting cards
val InfoPillFill = Color(0x78000000)            // rgba(0,0,0,0.47) info pill fill

// "Sync Location Now" refresh action — dark rounded container behind the icon
val SyncButtonFill = Color(0xFF2A2A2A)          // refresh action container fill #2A2A2A







/**
 * Vertical background gradient used across onboarding screens,
 * mirroring the Figma frame fill.
 */
val OnboardingBackgroundBrush = Brush.verticalGradient(
    colors = listOf(BackgroundGradientTop, BackgroundGradientBottom)
)
