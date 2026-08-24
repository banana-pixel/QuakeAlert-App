package id.web.quakealert.ui.app

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.data.network.ServerHealth
import kotlinx.coroutines.flow.StateFlow

/**
 * Exposes the one global [ServerHealth] verdict to the UI, for
 * [id.web.quakealert.ui.main.MainScreen] to hoist into every tab's
 * [id.web.quakealert.ui.common.QuakeAppBar].
 *
 * A ViewModel of its own rather than a field on each screen's ViewModel, because
 * server health belongs to no single tab: one owner is what makes the badge
 * impossible to derive two different ways. The flow is passed straight through —
 * the monitor's [ServerHealth] is already a process-lifetime [StateFlow] whose
 * polling starts only while a screen is subscribed, so there is nothing here to
 * re-share or re-scope.
 */
class ServerHealthViewModel(application: Application) : AndroidViewModel(application) {

    val health: StateFlow<ServerHealth> =
        QuakeNetwork.from(application).serverHealthMonitor.health
}
