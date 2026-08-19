package id.web.quakealert.ui.common

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import id.web.quakealert.R
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.SwitchThumbActive
import id.web.quakealert.ui.theme.SwitchThumbInactive
import id.web.quakealert.ui.theme.SwitchTrackActive
import id.web.quakealert.ui.theme.SwitchTrackInactive

/**
 * Custom flat, dark toggle switch (Figma node 1:857) used by the Settings
 * setting rows. Unlike the default
 * Material 3 [androidx.compose.material3.Switch], this renders a subtle dark
 * track with a white thumb that grows and carries a small check glyph when
 * enabled (and a small cross when off), matching the QuakeAlert dark tokens.
 *
 * Fully stateless: the checked state and its toggle handler are hoisted to the
 * owning card / ViewModel.
 *
 * @param checked whether the switch is on.
 * @param onCheckedChange invoked with the new value when the track is tapped.
 */
@Composable
fun QuakeSwitch(
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier
) {
    val trackColor by animateColorAsState(
        targetValue = if (checked) SwitchTrackActive else SwitchTrackInactive,
        animationSpec = tween(durationMillis = 180),
        label = "SwitchTrackColor"
    )
    val thumbColor by animateColorAsState(
        targetValue = if (checked) SwitchThumbActive else SwitchThumbInactive,
        animationSpec = tween(durationMillis = 180),
        label = "SwitchThumbColor"
    )
    val thumbSize by animateDpAsState(
        targetValue = if (checked) Dimens.SwitchThumbActiveSize else Dimens.SwitchThumbInactiveSize,
        animationSpec = tween(durationMillis = 180),
        label = "SwitchThumbSize"
    )
    val alignment = if (checked) Alignment.CenterEnd else Alignment.CenterStart
    val interactionSource = remember { MutableInteractionSource() }

    Box(
        modifier = modifier
            .size(width = Dimens.SwitchWidth, height = Dimens.SwitchHeight)
            .clip(CircleShape)
            .background(trackColor, CircleShape)
            .clickable(
                interactionSource = interactionSource,
                indication = null,
                role = Role.Switch,
                onClick = { onCheckedChange(!checked) }
            )
            .padding(Dimens.SwitchPadding),
        contentAlignment = alignment
    ) {
        Box(
            modifier = Modifier
                .size(thumbSize)
                .clip(CircleShape)
                .background(thumbColor, CircleShape),
            contentAlignment = Alignment.Center
        ) {
            // Active thumb carries a check glyph; inactive thumb carries a small
            // cross (X), matching the Figma "Light Mode (Beta)" off state.
            Icon(
                painter = painterResource(
                    id = if (checked) R.drawable.ic_check else R.drawable.ic_close
                ),
                contentDescription = null,
                tint = if (checked) SwitchTrackActive else SwitchTrackInactive,
                modifier = Modifier.size(Dimens.SwitchThumbIconSize)
            )
        }
    }
}
