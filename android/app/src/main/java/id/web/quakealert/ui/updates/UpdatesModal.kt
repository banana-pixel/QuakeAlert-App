package id.web.quakealert.ui.updates

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.R
import id.web.quakealert.ui.common.QuakeEmptyState
import id.web.quakealert.ui.common.QuakeErrorState
import id.web.quakealert.ui.common.QuakeLoadingState
import id.web.quakealert.ui.common.QuakeModalHeader
import id.web.quakealert.ui.common.QuakePill
import id.web.quakealert.ui.theme.AboutModalGradient
import id.web.quakealert.ui.theme.CardBorder
import id.web.quakealert.ui.theme.CardSubtitle
import id.web.quakealert.ui.theme.CardSurface
import id.web.quakealert.ui.theme.CardTitle
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextSecondary

/**
 * The Updates overlay, hosted in its own [Dialog] window like the About modal: it
 * sits over the whole Settings screen, and both a back press and an outside tap
 * dismiss it, so navigation is never trapped.
 *
 * Stateful entry point — it owns the [UpdatesViewModel], which loads on construction.
 * Opening the overlay is therefore the request, and closing it is the only lifecycle
 * this list has.
 */
@Composable
fun UpdatesModalDialog(
    onDismiss: () -> Unit,
    viewModel: UpdatesViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        UpdatesModal(
            uiState = uiState,
            onDismiss = onDismiss,
            onRetry = viewModel::refresh,
            modifier = Modifier.padding(Dimens.ScreenHorizontalPadding)
        )
    }
}

/**
 * Stateless Updates card: the shared [QuakeModalHeader] over one of the three
 * outcomes — in flight, a list, or a failure.
 *
 * The list is a [LazyColumn] with a bounded height rather than a scrolling [Column]:
 * announcements are unbounded in count where the About copy is fixed, and the bound
 * is what keeps the card from growing past the screen on a long history.
 *
 * An empty result is rendered as an empty state, never as an error. Operators
 * publishing nothing is the normal condition of this screen, and the copy says so
 * instead of implying something failed.
 *
 * Exposed separately from [UpdatesModalDialog] so it can be previewed and tested
 * without a dialog window or a ViewModel.
 */
@Composable
fun UpdatesModal(
    uiState: UpdatesUiState,
    onDismiss: () -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.RadiusCard)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(AboutModalGradient, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(Dimens.ModalPadding),
        verticalArrangement = Arrangement.spacedBy(Dimens.UpdatesModalSectionGap)
    ) {
        QuakeModalHeader(onDismiss = onDismiss, title = "Updates")

        when {
            uiState.isLoading && uiState.updates.isEmpty() -> QuakeLoadingState(
                modifier = Modifier.height(Dimens.UpdatesModalStateHeight),
                message = "Loading updates..."
            )

            uiState.error != null -> QuakeErrorState(copy = uiState.error, onRetry = onRetry)

            uiState.isEmpty -> QuakeEmptyState(
                icon = R.drawable.ic_info_circle,
                message = "No Updates Yet",
                subtitle = "Announcements from the QuakeAlert team will appear here. " +
                    "Earthquake warnings are never sent this way."
            )

            else -> LazyColumn(
                modifier = Modifier.heightIn(max = Dimens.UpdatesModalListMaxHeight),
                verticalArrangement = Arrangement.spacedBy(Dimens.UpdatesModalItemGap)
            ) {
                items(items = uiState.updates, key = { it.id }) { item ->
                    OperatorUpdateCard(item = item)
                }
            }
        }
    }
}

/**
 * One announcement.
 *
 * The scope pill and the age share the footer because together they answer the only
 * two questions a notice raises before it is read: why it reached this phone, and
 * whether it is still current.
 */
@Composable
private fun OperatorUpdateCard(
    item: OperatorUpdateItem,
    modifier: Modifier = Modifier
) {
    val shape = RoundedCornerShape(Dimens.SettingCardRadius)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(CardSurface, shape)
            .border(Dimens.BorderThin, CardBorder, shape)
            .padding(Dimens.SettingCardPaddingHorizontal),
        verticalArrangement = Arrangement.spacedBy(Dimens.SettingCardTitleGap)
    ) {
        Text(text = item.title, style = CardTitle)
        Text(text = item.body, style = CardSubtitle, color = TextSecondary)

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(Dimens.SettingCardContentGap),
            verticalAlignment = Alignment.CenterVertically
        ) {
            QuakePill(text = item.scope)
            Text(text = item.published, style = CardSubtitle, color = TextSecondary)
        }
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun UpdatesModalPreview() {
    QuakeAlertTheme {
        UpdatesModal(
            uiState = UpdatesUiState(
                updates = listOf(
                    OperatorUpdateItem(
                        id = "1",
                        title = "Scheduled maintenance",
                        body = "Sensor data may lag by a few minutes tonight between " +
                            "01:00 and 02:00 WIB.",
                        scope = "Nationwide",
                        published = "2 hours ago"
                    ),
                    OperatorUpdateItem(
                        id = "2",
                        title = "Drill on Friday",
                        body = "A regional preparedness drill runs on Friday morning.",
                        scope = "Jawa Barat",
                        published = "1 day ago"
                    )
                )
            ),
            onDismiss = {},
            onRetry = {}
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun UpdatesModalEmptyPreview() {
    QuakeAlertTheme {
        UpdatesModal(uiState = UpdatesUiState(), onDismiss = {}, onRetry = {})
    }
}
