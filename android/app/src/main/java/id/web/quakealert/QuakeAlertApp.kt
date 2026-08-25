package id.web.quakealert

import android.app.Application
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import id.web.quakealert.data.network.QuakeNetwork
import id.web.quakealert.device.BackgroundAlertBridge
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob

/**
 * Process-wide initialisation.
 *
 * Wires [BackgroundAlertBridge] to the process lifecycle so WebSocket alerts that
 * arrive while no activity is visible are raised as system notifications instead of
 * disappearing into a paused ViewModel — the failure mode where a backgrounded app
 * consumed the alert, marked dedup, and left the user with nothing on screen.
 */
class QuakeAlertApp : Application() {

    private val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    private val lifecycleObserver = object : DefaultLifecycleObserver {
        override fun onStart(owner: LifecycleOwner) {
            BackgroundAlertBridge.onForeground()
        }

        override fun onStop(owner: LifecycleOwner) {
            BackgroundAlertBridge.onBackground()
        }
    }

    override fun onCreate() {
        super.onCreate()

        ProcessLifecycleOwner.get().lifecycle.addObserver(lifecycleObserver)
        BackgroundAlertBridge.attach(this, appScope)

        // Touch QuakeNetwork early so its singletons (dedup, socket) exist before the
        // first frame can arrive through either channel.
        QuakeNetwork.from(this)
    }
}
