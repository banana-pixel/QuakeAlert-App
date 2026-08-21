package id.web.quakealert.ui.common

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.platform.LocalInspectionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.Dp
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.SkeletonBase
import id.web.quakealert.ui.theme.SkeletonHighlight

/**
 * Paints the receiver as an animated skeleton block: a [SkeletonBase] fill with a
 * [SkeletonHighlight] band sweeping across it.
 *
 * Used instead of a lone spinner for a list's *first* load. A spinner says "wait";
 * a skeleton says "wait, and here is the shape of what is coming", which on a
 * screen whose cards are all the same height keeps the layout from jumping when the
 * real rows arrive. Deliberately not used for refresh or for appending a page —
 * those already have content on screen, and replacing it with grey blocks would
 * look like the data was lost.
 *
 * The sweep runs off both edges of the element (`-size` to `2 × size`) so there is
 * no visible moment where the band sits still at one end.
 *
 * @param shape clip applied before painting, so the sweep follows the block's
 *   corners rather than a rectangle behind them.
 * @param durationMillis one full sweep. Slow on purpose: a fast shimmer reads as
 *   an error state flashing.
 */
fun Modifier.shimmer(
    shape: RoundedCornerShape = RoundedCornerShape(Dimens.SkeletonRadius),
    durationMillis: Int = SHIMMER_DURATION_MS
): Modifier = composed {
    // In a @Preview or a layout inspector there is no frame clock driving an
    // infinite animation to a stable state, so the tooling would render forever.
    // A still block is the honest preview of a skeleton anyway.
    if (LocalInspectionMode.current) {
        return@composed clip(shape).background(SkeletonBase)
    }

    val transition = rememberInfiniteTransition(label = "shimmer")
    val progress by transition.animateFloat(
        initialValue = 0f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = durationMillis, easing = LinearEasing),
            repeatMode = RepeatMode.Restart
        ),
        label = "shimmerProgress"
    )

    clip(shape).drawWithCache {
        val sweep = size.width * SWEEP_SPAN
        val start = -sweep + progress * (size.width + 2 * sweep)
        val brush = Brush.linearGradient(
            colors = listOf(SkeletonBase, SkeletonHighlight, SkeletonBase),
            start = Offset(start, 0f),
            end = Offset(start + sweep, size.height)
        )
        onDrawBehind { drawRect(brush = brush) }
    }
}

/**
 * A single skeleton line, sized as a fraction of the available width so a stack of
 * them reads as text of varying length rather than as identical bars.
 */
@Composable
fun SkeletonLine(
    modifier: Modifier = Modifier,
    widthFraction: Float = 1f,
    height: Dp = Dimens.SkeletonLineHeight
) {
    Box(
        modifier = modifier
            .fillMaxWidth(widthFraction)
            .height(height)
            .shimmer()
    )
}

/**
 * One placeholder card standing in for a not-yet-loaded row.
 *
 * Sized to [Dimens.CardHeight] — the height every real list card uses — so the
 * loading list occupies exactly the space the loaded one will.
 */
@Composable
fun QuakeSkeletonCard(modifier: Modifier = Modifier) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .height(Dimens.CardHeight)
            .clip(RoundedCornerShape(Dimens.RadiusCard))
            .background(SkeletonBase)
            .padding(
                start = Dimens.CardPaddingStart,
                top = Dimens.CardPaddingTop,
                end = Dimens.CardPaddingEnd,
                bottom = Dimens.CardPaddingBottom
            ),
        verticalArrangement = Arrangement.spacedBy(Dimens.SkeletonLineGap)
    ) {
        SkeletonLine(widthFraction = 0.45f)
        SkeletonLine(widthFraction = 0.8f)
        SkeletonLine(widthFraction = 0.6f)
    }
}

/**
 * A screenful of [QuakeSkeletonCard]s, shown in place of a list's first load.
 *
 * Marked as one decorative node for accessibility: a screen reader announcing five
 * identical empty cards is worse than silence, so the whole block carries a single
 * spoken [loadingLabel] and its children are invisible to semantics.
 *
 * @param loadingLabel what a screen reader says while this is up — the screen's own
 *   loading copy, so the announcement matches what a sighted user is told.
 */
@Composable
fun QuakeSkeletonList(
    loadingLabel: String,
    modifier: Modifier = Modifier,
    count: Int = Dimens.SkeletonCardCount
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .semantics(mergeDescendants = true) { contentDescription = loadingLabel },
        verticalArrangement = Arrangement.spacedBy(Dimens.CardListSpacing)
    ) {
        repeat(count) { QuakeSkeletonCard() }
    }
}

/** One full sweep of the highlight band, in milliseconds. */
const val SHIMMER_DURATION_MS = 1_200

/** Width of the highlight band as a fraction of the element it sweeps across. */
private const val SWEEP_SPAN = 0.6f

@Preview(showBackground = true, backgroundColor = 0xFF0A0A0A)
@Composable
private fun QuakeSkeletonListPreview() {
    QuakeAlertTheme {
        QuakeSkeletonList(
            loadingLabel = "Loading earthquake history",
            count = 2,
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}
