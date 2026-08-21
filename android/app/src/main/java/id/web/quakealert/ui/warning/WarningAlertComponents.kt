package id.web.quakealert.ui.warning

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.tooling.preview.Preview
import id.web.quakealert.R
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.EmergencyAlertGradient
import id.web.quakealert.ui.theme.EmergencyAlertIconBadgeFill
import id.web.quakealert.ui.theme.EmergencyAlertTitle
import id.web.quakealert.ui.theme.EmergencyControlBorder
import id.web.quakealert.ui.theme.EmergencyControlFillEngaged
import id.web.quakealert.ui.theme.EmergencyControlFillIdle
import id.web.quakealert.ui.theme.EmergencyControlLabel
import id.web.quakealert.ui.theme.EmergencyIntensityLabel
import id.web.quakealert.ui.theme.EmergencyIntensityValue
import id.web.quakealert.ui.theme.EmergencyProximity
import id.web.quakealert.ui.theme.EmergencySosFillIdle
import id.web.quakealert.ui.theme.EmergencySosLabel
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SuggestedActionsFill
import id.web.quakealert.ui.theme.SuggestedActionsHeader
import id.web.quakealert.ui.theme.TextPrimary

/**
 * The seismic emergency card (Figma node 1:1058) — everything the Warning screen
 * shows while a quake is being detected, and nothing else.
 *
 * Four blocks, distributed with `space-between` over the card's full height exactly
 * as the design does, so the most important read-out sits at the optical centre and
 * the two controls stay pinned within thumb reach at the bottom:
 *  1. the alert badge + "Earthquake Alert" headline,
 *  2. the estimated intensity and the proximity line,
 *  3. the "Suggested Actions :" box with the Drop / Cover / Hold On pictograms,
 *  4. the MUTE ALERT and SOS LIGHT hardware controls.
 *
 * Stateless: [state] is rendered as given and every tap leaves through
 * [onMuteClick] / [onSosLightClick] to [WarningViewModel], which owns the siren and
 * the torch. Nothing here starts a hardware effect, which is what lets this card be
 * previewed and tested without a device.
 *
 * One design effect is deliberately not reproduced: Figma puts a white 30dp glow
 * behind the intensity block (`0 4 30 rgba(255,255,255,0.58)`). Compose has no
 * blurred shadow for arbitrary content below API 31, and the available substitutes
 * (a translucent scrim, a blurred copy) read as a smudge over the gradient rather
 * than as a glow. The block carries the design's weight through its type scale
 * instead — Black 24 against the card's Bold 16.
 */
@Composable
fun ActiveAlertCard(
    state: WarningUiState.ActiveAlert,
    onMuteClick: () -> Unit,
    onSosLightClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .background(EmergencyAlertGradient, RoundedCornerShape(Dimens.EmergencyCardRadius))
            .border(
                width = Dimens.BorderThin,
                color = CardBorder,
                shape = RoundedCornerShape(Dimens.EmergencyCardRadius)
            )
            .padding(Dimens.EmergencyCardPadding),
        verticalArrangement = Arrangement.SpaceBetween,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        AlertHeadline()

        IntensityReadout(
            intensityValue = state.intensityValue,
            proximityLabel = state.proximityLabel
        )

        SuggestedActionsBox()

        EmergencyControls(
            isMuted = state.isMuted,
            isSosLightOn = state.isSosLightOn,
            isSosLightUnavailable = state.isSosLightUnavailable,
            onMuteClick = onMuteClick,
            onSosLightClick = onSosLightClick
        )
    }
}

/** Badge + headline block (Figma node 1:1059). */
@Composable
private fun AlertHeadline(modifier: Modifier = Modifier) {
    Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(Dimens.EmergencyBadgeTitleGap),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Column(
            modifier = Modifier
                .size(Dimens.EmergencyIconBadgeSize)
                .background(EmergencyAlertIconBadgeFill, CircleShape),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Icon(
                // The 63dp-viewport twin of ic_alert_triangle: the 24dp icon scaled
                // 2.6x here would draw its stroke as a hairline.
                painter = painterResource(id = R.drawable.ic_alert_triangle_lg),
                contentDescription = null,
                tint = TextPrimary,
                modifier = Modifier.size(Dimens.EmergencyIconGlyphSize)
            )
        }

        Text(text = "Earthquake Alert", style = EmergencyAlertTitle)
    }
}

/**
 * Intensity + proximity block (Figma node 1:1064).
 *
 * The label is a separate line from the value on purpose: it is the card's one
 * quantitative read, and a user glancing at a shaking phone should be able to take
 * the value in without parsing a sentence around it.
 */
@Composable
private fun IntensityReadout(
    intensityValue: String,
    proximityLabel: String,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(Dimens.EmergencyReadoutGap),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(Dimens.EmergencyIntensityGap),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(
                text = "Estimated Intensity :",
                style = EmergencyIntensityLabel,
                modifier = Modifier.fillMaxWidth()
            )
            Text(
                text = intensityValue,
                style = EmergencyIntensityValue,
                modifier = Modifier.fillMaxWidth()
            )
        }

        Text(
            text = proximityLabel,
            style = EmergencyProximity,
            modifier = Modifier.fillMaxWidth()
        )
    }
}

/**
 * "Suggested Actions :" container (Figma node 1:1069) holding the three official
 * Drop-Cover-Hold-On pictograms.
 *
 * The design ships the trio as one raster; it is rendered here as three cards, one
 * per [SuggestedAction], so each panel can carry its own `contentDescription` — a
 * single image would announce itself to a screen reader as one unlabelled graphic,
 * which for the card's only actual instruction is the wrong outcome. See
 * [SuggestedAction] for why the wordmarks stay baked into the artwork.
 */
@Composable
private fun SuggestedActionsBox(modifier: Modifier = Modifier) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(
                SuggestedActionsFill,
                RoundedCornerShape(Dimens.SuggestedActionsRadius)
            )
            .border(
                width = Dimens.BorderThin,
                color = CardBorder,
                shape = RoundedCornerShape(Dimens.SuggestedActionsRadius)
            )
            .padding(
                top = Dimens.SuggestedActionsPaddingTop,
                bottom = Dimens.SuggestedActionsPaddingBottom,
                start = Dimens.SuggestedActionsPaddingHorizontal,
                end = Dimens.SuggestedActionsPaddingHorizontal
            ),
        verticalArrangement = Arrangement.spacedBy(Dimens.SuggestedActionsHeaderGap),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text(
            text = "Suggested Actions :",
            style = SuggestedActionsHeader,
            modifier = Modifier.fillMaxWidth()
        )

        Row(
            modifier = Modifier
                .widthIn(max = Dimens.SuggestedActionsRowMaxWidth)
                .fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(Dimens.SuggestedActionCardGap)
        ) {
            suggestedActions().forEach { action ->
                SuggestedActionCard(
                    action = action,
                    modifier = Modifier.weight(1f)
                )
            }
        }
    }
}

/** One Drop / Cover / Hold On panel (sliced from the Figma raster 1:1071). */
@Composable
private fun SuggestedActionCard(
    action: SuggestedAction,
    modifier: Modifier = Modifier
) {
    Image(
        painter = painterResource(id = action.pictogram),
        // The label, not "pictogram": the drawing is the instruction, so a screen
        // reader must read out "Drop!" rather than describe an illustration.
        contentDescription = action.label,
        // Square by source, and Fit rather than Crop so no figure loses its feet to
        // a rounding difference between the three panels.
        contentScale = ContentScale.Fit,
        modifier = modifier
            .aspectRatio(1f)
            .clip(RoundedCornerShape(Dimens.SuggestedActionCardRadius))
    )
}

/**
 * The two hardware controls (Figma node 1:1072): a wide MUTE ALERT button and a
 * square SOS LIGHT button, bottom-aligned so their different heights share a
 * baseline.
 *
 * Both are `clickable` rows rather than Material `Button`s, because the design
 * specifies its own fill/stroke/shape at every state and a themed button would
 * fight it with elevation and ripple colours of its own.
 */
@Composable
private fun EmergencyControls(
    isMuted: Boolean,
    isSosLightOn: Boolean,
    isSosLightUnavailable: Boolean,
    onMuteClick: () -> Unit,
    onSosLightClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(Dimens.EmergencyControlsGap),
        verticalAlignment = Alignment.Bottom
    ) {
        MuteControl(
            isMuted = isMuted,
            onClick = onMuteClick,
            modifier = Modifier.weight(1f)
        )

        SosLightControl(
            isOn = isSosLightOn,
            isUnavailable = isSosLightUnavailable,
            onClick = onSosLightClick
        )
    }
}

/**
 * MUTE ALERT control (Figma node 1:1073).
 *
 * A toggle: once muted it offers to restore the sound, with the glyph and label
 * swapping to say so. The alternative — a control that stays "MUTE ALERT" after
 * muting — leaves the user unable to tell whether the tap registered, on the one
 * screen where that uncertainty matters most.
 */
@Composable
private fun MuteControl(
    isMuted: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val label = if (isMuted) "SOUND ON" else "MUTE ALERT"
    Row(
        modifier = modifier
            .minimumInteractiveComponentSize()
            .background(
                color = if (isMuted) EmergencyControlFillEngaged else EmergencyControlFillIdle,
                shape = RoundedCornerShape(Dimens.EmergencyControlRadius)
            )
            .border(
                width = Dimens.EmergencyControlBorderWidth,
                color = EmergencyControlBorder,
                shape = RoundedCornerShape(Dimens.EmergencyControlRadius)
            )
            .clickable(role = Role.Button, onClickLabel = label, onClick = onClick)
            .padding(
                horizontal = Dimens.EmergencyMutePaddingHorizontal,
                vertical = Dimens.EmergencyMutePaddingVertical
            ),
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Icon(
            painter = painterResource(
                id = if (isMuted) R.drawable.ic_volume_max else R.drawable.ic_volume_x
            ),
            contentDescription = null,
            tint = TextPrimary,
            modifier = Modifier.size(Dimens.EmergencyMuteIconSize)
        )
        Text(
            text = label,
            style = EmergencyControlLabel,
            modifier = Modifier.padding(start = Dimens.EmergencyControlIconGap)
        )
    }
}

/**
 * SOS LIGHT control (Figma node 1:1076), toggling the torch's Morse SOS strobe.
 *
 * [isUnavailable] gets its own label rather than a disabled state: the control stays
 * tappable because the reason a torch refused (another app holding the camera) can
 * clear between one tap and the next, and a greyed-out button would tell the user
 * to stop trying.
 */
@Composable
private fun SosLightControl(
    isOn: Boolean,
    isUnavailable: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val label = when {
        isUnavailable -> "NO LIGHT"
        isOn -> "SOS ON"
        else -> "SOS LIGHT"
    }
    Column(
        modifier = modifier
            .size(Dimens.EmergencySosSize)
            .background(
                color = if (isOn) EmergencyControlFillEngaged else EmergencySosFillIdle,
                shape = RoundedCornerShape(Dimens.EmergencyControlRadius)
            )
            .border(
                width = Dimens.EmergencyControlBorderWidth,
                color = EmergencyControlBorder,
                shape = RoundedCornerShape(Dimens.EmergencyControlRadius)
            )
            .clickable(role = Role.Button, onClickLabel = label, onClick = onClick)
            .padding(Dimens.EmergencySosPadding),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Icon(
            painter = painterResource(id = R.drawable.ic_flashlight_on),
            contentDescription = null,
            tint = TextPrimary,
            modifier = Modifier.size(Dimens.EmergencySosIconSize)
        )
        Text(
            text = label,
            style = EmergencySosLabel,
            modifier = Modifier.padding(top = Dimens.EmergencyControlIconGap)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000, heightDp = 700)
@Composable
private fun ActiveAlertCardPreview() {
    QuakeAlertTheme {
        ActiveAlertCard(
            state = previewActiveAlert,
            onMuteClick = {},
            onSosLightClick = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000, heightDp = 700)
@Composable
private fun ActiveAlertCardEngagedPreview() {
    QuakeAlertTheme {
        ActiveAlertCard(
            state = previewActiveAlert.copy(isMuted = true, isSosLightOn = true),
            onMuteClick = {},
            onSosLightClick = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000, heightDp = 700)
@Composable
private fun ActiveAlertCardUnknownDistancePreview() {
    QuakeAlertTheme {
        ActiveAlertCard(
            state = previewActiveAlert.copy(
                distanceKm = null,
                locationName = "",
                isSosLightUnavailable = true
            ),
            onMuteClick = {},
            onSosLightClick = {}
        )
    }
}

/** Shared fixture for the emergency-card previews, matching the Figma content. */
internal val previewActiveAlert = WarningUiState.ActiveAlert(
    eventId = "evt_preview",
    intensityValue = "IV (moderate)",
    distanceKm = 3,
    locationName = "Bandung, West Java, ID"
)
