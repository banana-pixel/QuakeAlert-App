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
    /** 20x20 `filter-lines` glyph in the trailing filter-sheet trigger (Figma 1:714). */
    /** Leading glyph of a permissions-hub row. */
    val PermissionHubGlyphSize = 22.dp
    /** Check-circle in a satisfied permissions-hub row. */
    val PermissionHubBadgeGlyphSize = 16.dp
    val FilterTriggerGlyphSize = 20.dp
    /** Diameter of the dot marking "criteria are active" on that trigger. */
    val FilterTriggerBadgeSize = 8.dp
    /**
     * How far the list body follows the finger during a pull-to-refresh, at full
     * pull. Small on purpose: enough to feel elastic, not enough to hide a row.
     */
    val PullElasticDistance = 40.dp

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
    /** Gap between the intensity column and the details column (Figma 14). */
    val CardContentGap = 14.dp
    /**
     * Width of the leading intensity column (Figma width 45), shared with the
     * detail overlay's badge column so both start their text on the same line.
     */
    val CardLeadingColumnWidth = 45.dp

    // MMI circular badge
    val MmiBadgeSize = 45.dp
    val MmiBadgeBorder = 3.dp

    // Map thumbnail

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
    /**
     * Height of the same map inlined in the Settings "Sync Location Now" card. Short
     * on purpose: there it confirms where the last fix landed, and a full-height
     * preview would push the sync control below the fold.
     */
    val MapCardInlineHeight = 130.dp
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

    // --- Shared QuakePill capsule (History "km Away" + Sensor telemetry) -----
    /** Uniform capsule height for every QuakePill (Figma 22). */
    val PillHeight = 22.dp
    val PillPaddingHorizontal = 10.dp
    val PillPaddingVertical = 3.dp
    /** Leading connectivity dot diameter on a status QuakePill (feedback: 6dp). */
    val PillDotSize = 6.dp
    /** Gap between the status dot and its label. */
    val PillDotGap = 5.dp

    // --- Settings screen ----------------------------------------------------
    /** Vertical gap between the header block and the scrolling content (Figma 16). */
    val SettingsHeaderGap = 16.dp
    /** Vertical spacing between setting cards / section blocks (Figma gap 20). */
    val SettingsSectionSpacing = 20.dp
    /** Bottom padding below the last setting card (Figma padding-bottom 20). */
    val SettingsListBottomPadding = 20.dp

    // Section header pill (Figma node 1:856): hug-width slim stadium capsule,
    // height 23, padding 0x14, 1px white-10% stroke, fully-rounded stadium.
    val SectionHeaderPillHeight = 23.dp
    val SectionHeaderPillPaddingHorizontal = 14.dp
    val SectionHeaderPillRadius = 10.dp


    // Setting row card (EL-002b7d17). Standardized container so every card
    // (Coverage, Sync, Auto Sync, Test Sound, Light Mode, Language) shares
    // identical chrome: 16dp radius, 16h/14v inner padding, 1dp white-10% stroke.
    val SettingCardRadius = 16.dp
    /**
     * Fixed card height so every setting row (Coverage, Sync, Auto Sync, Test
     * Sound, Light Mode, Language) is byte-identical regardless of
     * its trailing control. Derived from the Figma card (node 1:868): 10dp top +
     * 36dp title line-height + 10dp bottom = 56dp.
     */
    val SettingCardHeight = 56.dp
    /** Uniform horizontal inner padding for every setting card (Figma 10). */
    val SettingCardPaddingHorizontal = 16.dp
    /** Uniform vertical inner padding for every setting card (Figma 10). */
    val SettingCardPaddingVertical = 10.dp

    /** Gap between a card's text column and its trailing control (Figma gap 12). */
    val SettingCardContentGap = 12.dp
    /** Inner gap between a card title and its sub-line (Figma gap 6). */
    val SettingCardTitleGap = 6.dp


    // Segmented toggle control pills (Coverage / Language). Figma has no outer
    // container — each option is a standalone bordered pill (node 1:873 / 1:913)
    // with a 2px white-30% stroke and a 12px radius.
    /** Gap between segmented pills (Figma gap 6). */
    val SegmentRowGap = 6.dp
    /** Pill inner padding (Figma layout_c024a102: 14 vertical / 8 horizontal). */
    val SegmentPillPaddingHorizontal = 8.dp
    val SegmentPillPaddingVertical = 14.dp
    /** Pill corner radius (Figma 12px). */
    val SegmentPillRadius = 12.dp
    /**
     * Width of the whole control, so every segmented setting on the screen has the
     * same geometry no matter how long its labels or its card title are.
     *
     * A fixed width rather than [androidx.compose.foundation.layout.fillMaxWidth]:
     * the control sits in the trailing slot of a [id.web.quakealert.ui.common.QuakeCard]
     * whose title column is weighted, and in a Row `fillMaxWidth` takes the card's
     * *whole* width, which crushed the title to nothing. Sized to hold "Imperial",
     * the longest label, with its pill padding intact.
     */
    val SegmentControlWidth = 172.dp


    // Sync-now refresh action — dark rounded container behind the icon
    val SyncButtonSize = 44.dp
    val SyncButtonRadius = 12.dp
    val SyncButtonIconSize = 24.dp


    // Info pill ("Last Sync : ...")
    val InfoPillHeight = 22.dp
    val InfoPillPaddingHorizontal = 8.dp
    val InfoPillPaddingVertical = 2.dp
    val InfoPillRadius = 10.dp

    // Sync-now refresh icon
    val SyncRefreshIconSize = 32.dp

    // Custom M3-style switch (EL-d7322fd0)
    val SwitchWidth = 52.dp
    val SwitchHeight = 32.dp
    val SwitchPadding = 4.dp
    val SwitchThumbActiveSize = 24.dp
    val SwitchThumbInactiveSize = 16.dp
    val SwitchThumbIconSize = 16.dp

    // About card CTA button
    val AboutButtonPaddingHorizontal = 60.dp
    val AboutButtonPaddingVertical = 8.dp
    val AboutButtonRadius = 12.dp

    // --- Shared modal / overlay chrome --------------------------------------
    // Every overlay card (About node 4:668, Earthquake Details node 123:1002)
    // ships the same 18dp inner padding and the same circular close button, so
    // that chrome is named for the pattern instead of for one screen. Card
    // surfaces reuse RadiusCard (14dp) + BorderThin/CardBorder.
    /** Overlay card inner padding (Figma 18). */
    val ModalPadding = 18.dp

    /**
     * Gap between the floating message card and the bottom navigation it floats
     * over, so the card reads as sitting above the bar rather than joined to it.
     */
    val ToastBottomGap = 12.dp

    // Circular close (X) button (Figma nodes 4:761 / 123:1006): a 24dp glyph in a
    // 10dp-padded container → 44dp footprint, 20dp radius.
    val ModalCloseSize = 44.dp
    val ModalCloseRadius = 20.dp
    val ModalCloseIconSize = 24.dp

    // --- About modal / overlay (Figma node 4:654; card node 4:668) -----------
    /** Vertical gap between the modal's stacked sections (Figma gap 26). */
    val AboutModalSectionGap = 26.dp

    // Concentric logo badge (Figma node 4:670: fixed 139-tall block). The outer
    // halo fills the block height exactly; the ring and core step down in even
    // ~14dp increments.
    val AboutModalLogoSize = 139.dp
    val AboutModalLogoRingSize = 112.dp
    val AboutModalLogoCoreSize = 84.dp
    /**
     * Render box for the seismograph glyph. `ic_recording_wave` draws its
     * waveform inside only the middle ~34% of its 165-unit canvas, so the painter
     * is laid out larger than the core disc to bring the *visible* waveform up to
     * ~40dp; the surrounding empty margin is clipped away by the disc.
     */
    val AboutModalLogoGlyphSize = 120.dp

    // Action buttons (Figma nodes 4:677 / 4:681 / 4:686). Geometry is the shared
    // ModalAction* chrome below; only the gap between the two rows is About's own.
    /** Gap between the two action rows and between the buttons on a row (Figma 20). */
    val AboutModalActionGap = 20.dp

    // --- Shared overlay action button ---------------------------------------
    // The About overlay's three actions (nodes 4:677 / 4:681 / 4:686) and the
    // Earthquake Details "Share" button (node 124:1085) are the same component:
    // a 34dp-tall capsule with 6dp side padding, RadiusSmall (10dp) corners and a
    // BorderMedium (2dp) BorderLight stroke, differing only in fill and label.
    val ModalActionHeight = 34.dp
    val ModalActionPaddingHorizontal = 6.dp

    // --- Earthquake Details overlay (Figma node 123:743; card node 123:1002) --
    // The card reuses ModalPadding (18dp) + RadiusCard (14dp) + BorderThin.
    /**
     * Single 18dp rhythm the overlay uses throughout: between the card's stacked
     * sections, across the banner's badge → text split, and between the three
     * metric cells. Figma specifies 18 for all three, so they share one token
     * rather than three identical ones.
     */
    val EventDetailSectionGap = 18.dp

    /** Fixed height of the banner's text block (Figma node 124:1135: 66). */
    val EventDetailBannerHeight = 66.dp
    /** Gap between the "MMI" caption and the badge below it (Figma node 124:1171: 10). */
    val EventDetailMmiColumnGap = 10.dp

    /** Map thumbnail card (Figma node 123:1028: fixed 120 tall, 12 padding). */
    val EventDetailMapHeight = 120.dp
    val EventDetailMapPadding = 12.dp
    /** Solid epicentre dot at the centre of the map's pulse rings. */
    val EventDetailMapCentroidSize = 8.dp

    /**
     * Seismic metric cell (Figma node 124:1115: fixed 62 tall, padding 0 6). The
     * label/value pair is centred in the cell with this gap; Figma stacks two
     * 22-tall rows whose 16px line boxes leave ~6dp of air between them.
     */
    val EventDetailMetricCellHeight = 62.dp
    val EventDetailMetricCellPaddingHorizontal = 6.dp
    val EventDetailMetricCellGap = 6.dp

    /**
     * Spatial info card (Figma node 124:1147: padding 15, gap 10). Each of its two
     * rows occupies a fixed 44dp block (node 124:1153) that Figma splits into two
     * 22dp halves for the label and value.
     */
    val EventDetailInfoPadding = 15.dp
    val EventDetailInfoGap = 10.dp
    val EventDetailInfoRowHeight = 44.dp

    // --- Chat screen --------------------------------------------------------
    /** Gap between the header block (title + channel card) and the message list (Figma 16). */
    val ChatHeaderGap = 16.dp
    /** Channel/network card height (Figma node 1:934: fixed 69). */
    val ChatChannelCardHeight = 69.dp
    /** Channel card inner padding (Figma padding 10 20). */
    val ChatChannelCardPaddingHorizontal = 20.dp
    val ChatChannelCardPaddingVertical = 10.dp
    /** Gap between the channel text block and the trailing switch icon (Figma gap 11). */
    val ChatChannelCardContentGap = 11.dp
    /** Gap between the channel globe icon and its title (Figma gap 5). */
    val ChatChannelIconGap = 5.dp
    /** Inner gap between the channel title and the "N users online" subtitle (Figma gap 4). */
    val ChatChannelTitleGap = 4.dp
    val ChatChannelGlobeWidth = 19.dp
    val ChatChannelGlobeHeight = 20.dp
    val ChatChannelSwitchIconSize = 22.dp

    /** Vertical spacing between message bubbles / date separators in the list. */
    val ChatMessageSpacing = 10.dp
    /** Top padding above the first bubble in the scrolling list (Figma padding 20). */
    val ChatListTopPadding = 20.dp
    /** Bottom padding below the last bubble, above the input bar. */
    val ChatListBottomPadding = 20.dp
    /** Max fraction of the row width a single bubble may occupy. */
    val ChatBubbleMaxWidthFraction = 0.78f

    // Message bubble
    val ChatBubblePaddingHorizontal = 10.dp
    val ChatBubblePaddingVertical = 5.dp
    /** Inner gap between the (optional) sender name and the message body (Figma gap 0 / tight). */
    val ChatBubbleContentGap = 2.dp
    /** Gap between the bubble body and its trailing timestamp (Figma gap 10). */
    val ChatBubbleTimeGap = 10.dp
    val ChatBubbleRadius = 10.dp

    // Date separator pill (Figma template EL-c963c95e)
    val ChatDateSeparatorHeight = 23.dp
    val ChatDateSeparatorPaddingHorizontal = 12.dp
    val ChatDateSeparatorRadius = 10.dp

    // Input bar (Figma node 1:1017)
    /** Gap between the text field and the send button (Figma gap 18). */
    val ChatInputRowGap = 18.dp
    /** Bottom padding below the input row (Figma padding-bottom 20). */
    val ChatInputBottomPadding = 20.dp
    val ChatInputFieldPaddingHorizontal = 14.dp
    val ChatInputFieldPaddingVertical = 12.dp
    val ChatInputFieldRadius = 10.dp
    /** Send button is a fixed 50×50 rounded square (Figma node 1:1020). */
    val ChatSendButtonSize = 50.dp
    val ChatSendButtonRadius = 10.dp
    val ChatSendIconSize = 30.dp

    // --- Warning screen (Figma node 1:1024) ---------------------------------
    /** Vertical gap between the top header block and the scrolling content. */
    val WarningHeaderGap = 16.dp
    /** Vertical spacing between the banner block, divider, tips and CTA. */
    val WarningSectionSpacing = 20.dp
    /**
     * Leading glyph of the offline notice above the alert banner. Smaller than the
     * banner's own 50dp glyph on purpose: the notice reports on the app's link to
     * the network, which must never out-shout the earthquake it is reporting about.
     */
    val OfflineNoticeGlyphSize = 20.dp
    /** Inner padding of that notice, tighter than a card's — it is a strip, not a panel. */
    val OfflineNoticePadding = 10.dp
    /** Gap between the notice's glyph, its text column and the retry capsule. */
    val OfflineNoticeGap = 10.dp
    /** Bottom padding below the last element in the scrolling list. */
    val WarningListBottomPadding = CardListSpacing

    // Alert banner card (Figma node 1:1035)
    /** Banner fixed height (Figma 162). */
    val AlertBannerHeight = 162.dp
    /** Banner inner padding (Figma 20). */
    val AlertBannerPadding = 20.dp
    val AlertBannerRadius = 14.dp
    /** Inner gap between the banner's text lines (Figma title-gap rhythm, tightened). */
    val AlertBannerTitleGap = 4.dp
    /**
     * Banner glyph size. Figma ships 74dp; rendered at 64dp so the glyph reads
     * balanced against the compact text block instead of dominating the card.
     * Both variants' glyphs fill this box at the same stroke weight, so the
     * active-alert waveform and resting globe read at the same visual size.
     */
    val AlertWaveIconSize = 64.dp
    /**
     * Gap and glyph size for the info affordance beside the banner title. Smaller
     * than the title's own type size on purpose: the affordance has to be findable
     * without competing with the headline it sits next to.
     */
    val AlertBannerInfoGap = 6.dp
    val AlertBannerInfoIconSize = 16.dp
    /** "SEE DETAILS" capsule width (Figma 101) + padding (Figma 8/10). */
    val AlertActionWidth = 101.dp
    val AlertActionPaddingHorizontal = 10.dp
    val AlertActionPaddingVertical = 8.dp
    val AlertActionRadius = 10.dp

    // Divider (Figma node 1:1036/1:1037)
    /** Short centered drag-handle bar width (Figma 100). */
    val WarningDividerWidth = 100.dp
    /** Drag-handle stroke weight (Figma 3px). */
    val WarningDividerThickness = 3.dp
    /** Vertical padding around the divider (Figma 10). */
    val WarningDividerPaddingVertical = 10.dp

    // Preparedness tips list (Figma node 1:1038)
    /** Vertical spacing between tip rows (Figma gap 20). */
    val PrepTipSpacing = 20.dp
    /** Gap between the section title and the first tip (Figma gap 20). */
    val PrepSectionGap = 20.dp
    /** Gap between a tip's icon circle and its text column (Figma gap 10). */
    val PrepTipContentGap = 10.dp
    /** Circular tip icon container diameter. */
    val PrepIconCircleSize = 55.dp
    /** Glyph size inside the tip icon circle (Figma 35). */
    val PrepIconGlyphSize = 35.dp
    /** Inner gap between a tip title and its description (Figma gap 2). */
    val PrepTipTextGap = 2.dp

    // Emergency bottom CTA (Figma node 1:1039)
    /** CTA fixed height (Figma 34). */
    val EmergencyCtaHeight = 34.dp
    val EmergencyCtaRadius = 10.dp

    // --- Active earthquake alert screen (Figma node 1:1043) ------------------
    // The emergency card replaces the whole resting body, so it owns its own
    // geometry rather than extending the banner's.

    /** Card inner padding (Figma 25) and corner radius (Figma 14). */
    val EmergencyCardPadding = 25.dp
    val EmergencyCardRadius = 14.dp
    /** Gap between the card and the header above it. */
    val EmergencyCardTopGap = 16.dp

    /** Alert-triangle badge disc (Figma 105) and the glyph inside it (Figma 62.5). */
    val EmergencyIconBadgeSize = 105.dp
    val EmergencyIconGlyphSize = 62.5.dp
    /** Gap between the badge and the "Earthquake Alert" headline (Figma gap 20). */
    val EmergencyBadgeTitleGap = 20.dp

    /** Gap between the intensity block's label and its value (Figma gap 2). */
    val EmergencyIntensityGap = 2.dp
    /** Gap between the intensity block and the proximity line (Figma gap 12). */
    val EmergencyReadoutGap = 12.dp

    // "Suggested Actions :" container (Figma node 1:1069)
    val SuggestedActionsRadius = 14.dp
    val SuggestedActionsPaddingTop = 14.dp
    val SuggestedActionsPaddingHorizontal = 12.dp
    val SuggestedActionsPaddingBottom = 20.dp
    /** Gap between the container header and the action cards (Figma gap 15). */
    val SuggestedActionsHeaderGap = 15.dp
    /**
     * Gap between the three action cards. Figma ships the trio as one 226dp raster
     * with ~4% internal gutters; rebuilt as three cards, that gutter lands here.
     */
    val SuggestedActionCardGap = 8.dp
    val SuggestedActionCardRadius = 6.dp
    /**
     * Width the three action cards share, matching the raster the design places
     * here (Figma 226). Capped rather than filled: the artwork is a fixed-aspect
     * pictogram trio, and letting it stretch to the card's full inner width would
     * blow the figures up out of proportion to everything around them.
     */
    val SuggestedActionsRowMaxWidth = 226.dp

    // Emergency hardware controls (Figma nodes 1:1072 / 1:1073 / 1:1076)
    /** Gap between the wide MUTE control and the square SOS control (Figma gap 23). */
    val EmergencyControlsGap = 23.dp
    val EmergencyControlRadius = 10.dp
    /**
     * Control stroke (Figma 2). Twice [BorderThin] on purpose: these two are the
     * only interactive targets on the emergency screen, and the heavier outline is
     * what separates them from the card's decorative 1dp edges.
     */
    val EmergencyControlBorderWidth = 2.dp
    /** MUTE control padding (Figma 6/10) and its glyph size (Figma 22.04). */
    val EmergencyMutePaddingVertical = 6.dp
    val EmergencyMutePaddingHorizontal = 10.dp
    val EmergencyMuteIconSize = 22.dp
    /** Gap between a control's glyph and its label (Figma gap 6). */
    val EmergencyControlIconGap = 6.dp
    /** SOS control is a fixed 76x76 square (Figma) with a 30dp torch glyph. */
    val EmergencySosSize = 76.dp
    val EmergencySosPadding = 8.dp
    val EmergencySosIconSize = 30.dp

    // --- Shared state feedback (loading / empty / error) ---------------------
    // Geometry for the generic QuakeLoadingState / QuakeEmptyState /
    // QuakeErrorState placeholders. Reuses the preparedness-tip icon circle
    // rhythm (55dp disc / 35dp glyph) so a zero-data screen reads as part of the
    // same system as the populated ones, one step larger because it stands alone
    // in the middle of an empty viewport.
    val StateIconCircleSize = 72.dp
    val StateIconGlyphSize = 32.dp
    /** Gap between the state glyph and its text block. */
    val StateContentGap = 16.dp
    /** Inner gap between a state message and its subtitle. */
    val StateTextGap = 6.dp
    /** Horizontal inset so long state copy never runs to the screen edge. */
    val StateBlockPadding = 24.dp
    /** Diameter of the centered loading spinner. */
    val StateSpinnerSize = 36.dp
    val StateSpinnerStroke = 3.dp
    /** Side padding of the error state's "Retry" action (height is ModalActionHeight). */
    val StateRetryPaddingHorizontal = 24.dp

    // Card chrome shared by the error / no-data / no-coverage states
    // (Figma node 148:1066). A card rather than bare centred copy: the same shape
    // then carries "we could not ask" and "the answer is nothing", so the two read
    // as one language instead of one looking like a broken screen.
    // Drawn 346x322 in Figma and held one step smaller here. At the designed size
    // the card pushes against the filter row it sits under and fills the viewport,
    // which makes an explanation of why there is no content look like content. The
    // proportions are kept; only the scale changes. Width is capped and height is a
    // floor, so long copy grows the card instead of clipping.
    val StateCardMaxWidth = 300.dp
    val StateCardMinHeight = 220.dp
    val StateCardPadding = 18.dp
    /** Standalone glyph, no icon circle in this design (Figma 148:1070 is 50x50). */
    val StateCardGlyphSize = 36.dp
    /** Width of the title column (Figma 148:1067) so titles break like the design. */
    val StateCardTitleWidth = 228.dp
    /** Radius of the soft white wash standing in for the message frame's glow. */
    val StateCardGlowRadius = 30.dp

    // --- Corner radii -------------------------------------------------------
    val RadiusSmall = 10.dp


    /** Fully-rounded stadium radius for capsule pills. */
    val RadiusStadium = 100.dp

    val RadiusCard = 14.dp
    val RadiusNavItem = 16.dp
    val RadiusNavBar = 22.dp
    val RadiusMmiBadge = 22.5.dp

    // --- Stroke weights -----------------------------------------------------
    val BorderThin = 1.dp
    /** 2px stroke used by the section header pill (Figma node 1:856). */
    val BorderMedium = 2.dp

    // Test Alert Sound modal (Figma node 144:1025)

    /**
     * START ↔ STOP gap (node 144:1061). 20dp, not the 23dp
     * [EmergencyControlsGap]: that gap separates MUTE from the square SOS button
     * on the live alert screen, and this row is a different composition.
     */
    val TestAlertActionsGap = 20.dp

    /** Fixed label box behind each action (nodes 144:1050 / 144:1055): 90×34. */
    val TestAlertActionWidth = 90.dp

    // Skeleton placeholders shown while a list loads (no Figma node)

    // ============================================================
    // MapLibre basemap (id.web.quakealert.ui.common.QuakeMap)
    // ============================================================

    /** Inset of the attribution label from the map card's own edges. */
    val MapAttributionInset = 6.dp

    /** Padding inside the attribution label's scrim capsule. */
    val MapAttributionPaddingHorizontal = 5.dp
    val MapAttributionPaddingVertical = 2.dp

    /** Corner radius of a skeleton block, matching [RadiusCard]'s family. */
    val SkeletonRadius = 8.dp
    /** Height of a skeleton text line. */
    val SkeletonLineHeight = 14.dp
    /** Gap between the stacked lines inside one skeleton card. */
    val SkeletonLineGap = 10.dp
    /** How many placeholder cards a loading list shows — one screenful. */
    const val SkeletonCardCount = 5


}

