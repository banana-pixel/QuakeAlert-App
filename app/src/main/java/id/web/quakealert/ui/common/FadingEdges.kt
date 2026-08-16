package id.web.quakealert.ui.common

import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalDensity
import id.web.quakealert.ui.theme.Dimens

/**
 * Shared soft fade at BOTH the top and bottom edges of a scrolling list, applied
 * uniformly across History and Sensors so their scroll behaviour is consistent.
 *
 * Implemented as an alpha mask rather than an opaque colour overlay (Rule C), so
 * it is fully theme/background agnostic: it erases the content's alpha at the
 * edges so the real background always shows through with no colour-mismatch seam.
 *
 * How it works:
 *  - [graphicsLayer] with [CompositingStrategy.Offscreen] renders the list into
 *    an offscreen buffer so subsequent draws can composite against it.
 *  - After [drawContent], two gradients are drawn with [BlendMode.DstIn], which
 *    keeps the destination (the list) only where the source alpha is non-zero.
 *    The gradients run transparent→opaque at the top and opaque→transparent at
 *    the bottom, carving smooth fades into the content's alpha channel.
 *
 * The top brush is hoisted via [remember] keyed on the density-resolved fade
 * height, so no `Brush`/`Color`/`List` allocations happen during the draw phase
 * (the lambda runs every scroll frame). The bottom brush is positioned from
 * `size.height` at draw time — a cheap primitive read.
 */
fun Modifier.fadingEdges(): Modifier = composed {
    val fadeHeightPx = with(LocalDensity.current) { Dimens.ListFadeHeight.toPx() }
    // Top mask: opaque (keep) below the fade, transparent (erase) at the very top.
    val topMask = remember(fadeHeightPx) {
        Brush.verticalGradient(
            colors = listOf(Color.Transparent, Color.Black),
            startY = 0f,
            endY = fadeHeightPx
        )
    }
    // Bottom mask stops are reused; the brush is rebuilt only when height changes.
    val bottomColors = remember { listOf(Color.Black, Color.Transparent) }

    this
        .graphicsLayer { compositingStrategy = CompositingStrategy.Offscreen }
        .drawWithContent {
            drawContent()
            // Erase alpha at the top edge.
            drawRect(
                brush = topMask,
                size = size.copy(height = fadeHeightPx),
                blendMode = BlendMode.DstIn
            )
            // Erase alpha at the bottom edge.
            drawRect(
                brush = Brush.verticalGradient(
                    colors = bottomColors,
                    startY = size.height - fadeHeightPx,
                    endY = size.height
                ),
                topLeft = Offset(0f, size.height - fadeHeightPx),
                size = size.copy(height = fadeHeightPx),
                blendMode = BlendMode.DstIn
            )
        }
}
