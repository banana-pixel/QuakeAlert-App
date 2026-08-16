package id.web.quakealert.ui.theme

import androidx.compose.ui.unit.dp

/**
 * Centralized spacing, sizing and corner-radius tokens extracted from the
 * QuakeAlert Figma design (History Page node 1:701). Composables must reference
 * these instead of hardcoding raw dp values, per the QuakeAlert design rules.
 */
object Dimens {
    // --- Screen scaffolding -------------------------------------------------
    /** Horizontal inset for the History content column (Figma padding 0 28). */
    val ScreenHorizontalPadding = 28.dp

    // --- Top bar / header ---------------------------------------------------
    /** Gap between the header row and the filter row (Figma gap 16). */
    val HeaderSectionGap = 16.dp
    /** Gap between the "Healthy" badge icon and its label (Figma gap 5). */
    val BadgeIconGap = 5.dp
    val BadgePaddingHorizontal = 5.dp
    val BadgePaddingVertical = 4.dp

    // --- Filter row ---------------------------------------------------------
    /** Gap between filter pills / calendar button (Figma gap 14). */
    val FilterRowGap = 14.dp
    val FilterPillPaddingHorizontal = 10.dp
    val FilterPillPaddingVertical = 6.dp
    val FilterPillHeight = 30.dp
    val CalendarButtonPadding = 5.dp

    // --- History list -------------------------------------------------------
    /** Vertical spacing between history cards (Figma gap 20). */
    val CardListSpacing = 20.dp
    /** Top padding above the first card in the scrolling list (Figma padding 20). */
    val CardListTopPadding = 20.dp
    /**
     * Bottom padding below the last card. Kept equal to [CardListSpacing] so the
     * gap between the final card and the navigation pill matches the vertical
     * rhythm between cards. Single source of truth for the list's bottom gap now
     * that the bottom bar no longer folds its top margin into innerPadding.
     */
    val CardListBottomPadding = CardListSpacing

    /** Height of the soft alpha-mask fade drawn at both edges of the list. */
    val ListFadeHeight = 16.dp




    // --- History card -------------------------------------------------------
    val CardHeight = 132.dp
    val CardPaddingTop = 14.dp
    val CardPaddingEnd = 17.dp
    val CardPaddingBottom = 16.dp
    val CardPaddingStart = 20.dp
    /** Gap between the intensity/map column and the details column (Figma 14). */
    val CardContentGap = 14.dp
    /** Width of the leading intensity + map column (Figma width 45). */
    val CardLeadingColumnWidth = 45.dp
    /** Gap inside the leading column between MMI badge and map (Figma gap 12). */
    val CardLeadingColumnGap = 12.dp

    // MMI circular badge
    val MmiBadgeSize = 45.dp
    val MmiBadgeBorder = 3.dp

    // Map thumbnail
    val MapThumbHeight = 45.dp

    // Details column
    val DetailTitleDistanceGap = 4.dp
    val DetailFooterGap = 14.dp

    // Distance badge
    val DistanceBadgeHeight = 22.dp
    val DistanceBadgePadding = 10.dp

    // Share button
    val ShareButtonWidth = 28.dp
    val ShareButtonHeight = 22.dp

    // See-more vertical accent bar (right side)
    val SeeMoreBarWidth = 21.dp

    // --- Sensors screen -----------------------------------------------------
    /** Vertical gap between the (map + filter) header block and the list (Figma 20). */
    val SensorsHeaderBlockGap = 20.dp
    /** Map preview card height (Figma height 294). */
    val MapCardHeight = 294.dp
    /** Inner padding of the map preview card (Figma padding 12). */
    val MapCardPadding = 12.dp
    /** Location pill height on the map (Figma height 23). */
    val MapLocationPillHeight = 23.dp
    val MapLocationPillPaddingHorizontal = 5.dp
    val MapLocationPillPaddingVertical = 2.dp
    val MapLocationPillGap = 4.dp
    /** "Range : ..." summary badge height (Figma height 22). */
    val MapRangeBadgeHeight = 22.dp
    val MapRangeBadgePaddingHorizontal = 5.dp
    val MapRangeBadgePaddingVertical = 2.dp
    /** Circular settings-shortcut button diameter (Figma width 64). */
    val MapSettingsShortcutSize = 64.dp
    val MapSettingsShortcutIconSize = 24.dp
    val MapPinIconSize = 16.dp

    // Sensor station card
    /** Gap between the sensor chip column and the details column (Figma gap 20). */
    val SensorCardContentGap = 20.dp
    /** Leading chip column width (Figma group width 50; widened so "MPU 6050" fits). */
    val SensorChipColumnWidth = 56.dp
    /** cpu-chip icon badge diameter (Figma frame ~45, fills column). */
    val SensorChipBadgeSize = 50.dp
    /** cpu-chip icon container corner radius (Figma 22.5). */
    val SensorChipRadius = 22.5.dp
    val SensorChipIconPadding = 8.dp
    val SensorChipIconSize = 24.dp
    /** Gap between the chip badge and the "MPU 6050" label (Figma gap 2). */
    val SensorChipLabelGap = 4.dp
    /** Details column fixed width (Figma width 216). */
    val SensorDetailsColumnWidth = 216.dp
    val SensorDetailsGap = 8.dp
    /** Gap between status/telemetry pills in a row (Figma gap 6). */
    val SensorChipRowGap = 6.dp
    /** Uniform pill height so status + telemetry pills align on a row. */
    val SensorPillHeight = 22.dp
    val SensorPillPaddingHorizontal = 10.dp
    /** Solid connectivity dot diameter on the status pill (feedback: 6dp). */
    val SensorStatusDotSize = 6.dp
    /** Gap between the status dot and its label. */
    val SensorStatusDotGap = 5.dp


    // --- Bottom navigation --------------------------------------------------

    val NavBarHeight = 71.dp
    val NavBarPaddingHorizontal = 8.dp
    val NavBarPaddingVertical = 10.dp
    val NavItemSize = 55.dp
    val NavItemGap = 4.dp
    val NavIconSize = 24.dp

    // --- Corner radii -------------------------------------------------------
    val RadiusSmall = 10.dp
    val RadiusCard = 14.dp
    val RadiusNavItem = 16.dp
    val RadiusNavBar = 22.dp
    val RadiusMmiBadge = 22.5.dp

    // --- Stroke weights -----------------------------------------------------
    val BorderThin = 1.dp
}
