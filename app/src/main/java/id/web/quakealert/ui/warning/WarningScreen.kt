package id.web.quakealert.ui.warning

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import id.web.quakealert.ui.common.QuakeAppBar
import id.web.quakealert.ui.common.fadingEdges
import id.web.quakealert.ui.theme.Dimens
import id.web.quakealert.ui.theme.NunitoFontFamily
import id.web.quakealert.ui.theme.QuakeAlertTheme
import id.web.quakealert.ui.theme.TextPrimary

/**
 * Stateful entry point that connects [WarningViewModel] to the stateless
 * [WarningScreen]. Kept thin so the presentation layer stays testable.
 */
@Composable
fun WarningRoute(
    modifier: Modifier = Modifier,
    viewModel: WarningViewModel = viewModel()
) {
    val uiState by viewModel.uiState.collectAsState()

    WarningScreen(
        uiState = uiState,
        onSeeDetails = viewModel::onSeeDetailsClicked,
        onEmergency = viewModel::onEmergencyClicked,
        modifier = modifier
    )
}

/**
 * Stateless Warning screen (Figma node 1:1024). Structure, top → bottom, mirrors
 * the Chat/History layout so all tabs share behaviour:
 *  1. A static header [Column] pinned to the top: shared [QuakeAppBar] + the
 *     active [AlertBanner] + a short [WarningDivider].
 *  2. A weighted [LazyColumn] carrying the "Preparedness Tips" section title and
 *     the tip rows, with the shared soft [fadingEdges] at the scroll bounds.
 *  3. A pinned [EmergencyCta] at the bottom.
 *
 * All state and events are hoisted to the caller ([WarningRoute] /
 * [WarningViewModel]).
 */
@Composable
fun WarningScreen(
    uiState: WarningUiState,
    onSeeDetails: () -> Unit,
    onEmergency: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = Dimens.ScreenHorizontalPadding)
    ) {
        // --- Static header: title + alert banner + divider -------------------
        QuakeAppBar(title = "Warning", isHealthy = uiState.isHealthy)

        AlertBanner(
            banner = uiState.banner,
            onSeeDetails = onSeeDetails,
            modifier = Modifier.padding(top = Dimens.WarningHeaderGap)
        )

        WarningDivider()

        // --- Scrolling preparedness tips -------------------------------------
        LazyColumn(
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                .fadingEdges(),
            contentPadding = PaddingValues(
                top = Dimens.PrepSectionGap,
                bottom = Dimens.WarningListBottomPadding
            ),
            verticalArrangement = Arrangement.spacedBy(Dimens.PrepTipSpacing)
        ) {
            item(key = "prep-title") {
                androidx.compose.material3.Text(
                    text = "Preparedness Tips",
                    color = TextPrimary,
                    fontFamily = NunitoFontFamily,
                    fontWeight = FontWeight.Bold,
                    fontSize = 18.sp,
                    lineHeight = 22.sp
                )
            }
            items(
                items = uiState.tips,
                key = { it.id }
            ) { tip ->
                PrepTipRow(tip = tip)
            }
        }

        // --- Pinned emergency CTA --------------------------------------------
        EmergencyCta(
            onClick = onEmergency,
            modifier = Modifier.padding(bottom = Dimens.WarningListBottomPadding)
        )
    }
}

@Preview(showBackground = true, backgroundColor = 0xFF000000)
@Composable
private fun WarningScreenPreview() {
    QuakeAlertTheme {
        WarningScreen(
            uiState = WarningUiState(),
            onSeeDetails = {},
            onEmergency = {}
        )
    }
}
