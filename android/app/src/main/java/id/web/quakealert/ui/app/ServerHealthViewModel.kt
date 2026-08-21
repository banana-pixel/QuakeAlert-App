package id.web.quakealert.ui.app

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.domain.ServerConnectionState
import kotlinx.coroutines.flow.StateFlow

/**
 * Exposes the one global [ServerConnectionState] to the UI, for
 * [id.web.quakealert.ui.main.MainScreen] to hoist into every tab's
 * [id.web.quakealert.ui.common.QuakeAppBar].
 *
 * A ViewModel of its own rather than a field on each screen's ViewModel, because
 * server health belongs to no single tab: one owner is what makes the badge
 * impossible to derive two different ways. The flow is passed straight through —
 * [id.web.quakealert.data.network.QuakeWebSocketClient.connectionState] is already
 * a process-lifetime [StateFlow], so there is nothing here to re-share or re-scope.
 */
class ServerHealthViewModel(application: Application) : AndroidViewModel(application) {

    val connectionState: StateFlow<ServerConnectionState> =
        QuakeNetwork.from(application).webSocketClient.connectionState
}
