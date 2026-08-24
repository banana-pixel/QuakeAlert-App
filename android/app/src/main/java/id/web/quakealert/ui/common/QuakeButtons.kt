package id.web.quakealert.ui.common

import androidx.compose.animation.core.animateDpAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.foundation.BorderStroke
import id.web.quakealert.ui.theme.AccentBlueTranslucent
import id.web.quakealert.ui.theme.BorderLight
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.OverlayLight
import id.web.quakealert.ui.theme.TextPrimary
import id.web.quakealert.ui.theme.TextSecondary
import id.web.quakealert.ui.theme.WizardActionLabel
import id.web.quakealert.ui.theme.WizardPanelStroke
import id.web.quakealert.ui.theme.WizardPrimaryActionFill

/**
 * The two CTA capsules and the animated page indicator shared by the multi-step
 * flows (Onboarding, Add-a-Sensor wizard).
 *
 * Extracted from OnboardingScreen's private composables verbatim so every flow's
 * primary action looks - and animates - identically; a second private copy per
 * screen is how "the same button" drifts into three different ones.
 */

/** Filled cyan/dark-blue CTA capsule (Start / Next / Register sensor). */
@Composable
fun QuakePrimaryButton(
    text: String,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    onClick: () -> Unit
) {
    Button(
        onClick = onClick,
        enabled = enabled,
        shape = RoundedCornerShape(40.dp),
        colors = ButtonDefaults.buttonColors(
            containerColor = AccentBlueTranslucent,
            contentColor = TextPrimary
        ),
        modifier = modifier
            .height(51.dp)
            .border(width = 3.dp, color = BorderLight, shape = RoundedCornerShape(40.dp))
    ) {
        Text(
            text = text,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 15.sp
        )
    }
}

/** Bordered, transparent-fill capsule (Back / Cancel). */
@Composable
fun QuakeSecondaryButton(
    text: String,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    onClick: () -> Unit
) {
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        shape = RoundedCornerShape(40.dp),
        colors = ButtonDefaults.outlinedButtonColors(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            contentColor = TextPrimary
        ),
        border = BorderStroke(3.dp, BorderLight),
        modifier = modifier.height(51.dp)
    ) {
        Text(
            text = text,
            color = TextPrimary,
            fontFamily = NunitoFontFamily,
            fontWeight = FontWeight.Bold,
            fontSize = 15.sp
        )
    }
}

/**
 * Determinate page indicator: a rounded track whose active white segment animates
 * to sit under the current page (anchored left).
 *
 * @param totalWidth track width; the default matches onboarding's design spec.
 */
@Composable
fun QuakePageIndicator(
    pageCount: Int,
    currentPage: Int,
    modifier: Modifier = Modifier,
    totalWidth: Dp = 100.dp
) {
    val segment = if (pageCount > 0) totalWidth / pageCount else totalWidth
    val activeWidth by animateDpAsState(targetValue = segment, label = "indicatorWidth")
    val offset by animateDpAsState(targetValue = segment * currentPage, label = "indicatorOffset")

    Box(
        modifier = modifier
            .width(totalWidth)
            .height(6.dp)
            .clip(RoundedCornerShape(100.dp))
            .background(OverlayLight)
    ) {
        Box(
            modifier = Modifier
                .padding(start = offset)
                .width(activeWidth)
                .height(6.dp)
                .clip(RoundedCornerShape(100.dp))
                .background(TextPrimary)
        )
    }
}

/**
 * The 34dp capsule every overlay uses for its own actions (About's three links,
 * the detail card's Share, the wizard's Back / Next pair): a flat fill, a 2dp
 * white-30% stroke and a bold centred label.
 *
 * Separate from [QuakePrimaryButton] rather than a variant of it: that one is a
 * 51dp full-bleed CTA sized for a screen, and an overlay action is a compact
 * capsule sized for a card. Disabled state dims the label instead of removing the
 * capsule, so the row keeps its shape while a step is busy.
 *
 * @param filled false draws the outlined variant used for the secondary choice.
 * @param container fill for the filled variant; pass the flow's own accent.
 */
@Composable
fun QuakeModalActionButton(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    filled: Boolean = true,
    container: Color = WizardPrimaryActionFill,
    enabled: Boolean = true
) {
    val shape = RoundedCornerShape(Dimens.RadiusSmall)
    Box(
        modifier = modifier
            .heightIn(min = Dimens.ModalActionHeight)
            .clip(shape)
            .background(if (filled) container else Color.Transparent, shape)
            .border(Dimens.BorderMedium, WizardPanelStroke, shape)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .padding(
                horizontal = Dimens.WizardActionPaddingHorizontal,
                vertical = Dimens.WizardFieldPaddingVertical
            ),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = label,
            style = WizardActionLabel,
            color = if (enabled) TextPrimary else TextSecondary,
            maxLines = 1
        )
    }
}
