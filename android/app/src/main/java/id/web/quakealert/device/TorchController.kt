package id.web.quakealert.device

import android.content.Context
import android.content.pm.PackageManager
import android.hardware.camera2.CameraAccessException
import android.hardware.camera2.CameraCharacteristics
import android.hardware.camera2.CameraManager
import android.util.Log
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * The device torch, driven as an SOS strobe for the Warning screen's "SOS LIGHT"
 * control (Figma node 1:1076).
 *
 * **No runtime permission is involved.** `CameraManager.setTorchMode` has been
 * permission-free since API 23 precisely so an app can signal without holding
 * `CAMERA` — and this app deliberately does not declare `CAMERA`, because asking
 * for camera access to blink a light would be a far larger claim on the user than
 * the feature needs. What *does* need handling is availability: a device may have
 * no flash unit at all, and the torch can be refused at any moment by whatever
 * else holds the camera. Both surface through [start]'s return value, so the UI can
 * tell the user the light did not come on instead of leaving a control that looks
 * engaged over a dark LED.
 *
 * The strobe is Morse **· · · — — — · · ·** rather than a steady beam: it is what a
 * searcher is looking for, and duty-cycling the LED also keeps it alive far longer
 * on a phone that may be the only light in a collapsed room.
 *
 * Not thread-safe — call it from the main thread (the ViewModel does).
 */
class TorchController(context: Context) {

    private val cameraManager: CameraManager? =
        context.getSystemService(Context.CAMERA_SERVICE) as? CameraManager

    private val hasFlashFeature: Boolean =
        context.packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_FLASH)

    /**
     * Id of the first flash-capable camera, resolved once. Null when the device has
     * no such camera, which also makes [isAvailable] false.
     */
    private val flashCameraId: String? by lazy { findFlashCameraId() }

    private var strobeJob: Job? = null

    /** True when this device can actually turn a torch on. */
    val isAvailable: Boolean
        get() = hasFlashFeature && flashCameraId != null

    /** True while the strobe loop is running. */
    val isOn: Boolean
        get() = strobeJob?.isActive == true

    /**
     * Starts the SOS strobe in [scope], returning false when the device has no
     * usable torch. Idempotent: a second call while running is a no-op rather than
     * a second loop fighting the first over the LED.
     *
     * The loop owns turning the LED off again — including on cancellation, via a
     * [NonCancellable] finally, because a cancelled coroutine that skipped its
     * cleanup would leave the torch burning with no UI left to switch it off.
     */
    fun start(scope: CoroutineScope): Boolean {
        if (isOn) return true
        val cameraId = flashCameraId?.takeIf { hasFlashFeature } ?: return false

        // Fail fast on a torch the system refuses right now, so the caller does not
        // light up an "engaged" control over a dark LED.
        if (!setTorch(cameraId, enabled = true)) return false

        strobeJob = scope.launch {
            try {
                while (true) {
                    for ((enabled, durationMs) in SOS_PATTERN) {
                        // A refusal mid-pattern (another app grabbed the camera) ends
                        // the strobe rather than spinning on a dead LED.
                        if (!setTorch(cameraId, enabled)) return@launch
                        delay(durationMs)
                    }
                }
            } finally {
                withContext(NonCancellable) { setTorch(cameraId, enabled = false) }
            }
        }
        return true
    }

    /** Stops the strobe and switches the LED off. Safe when nothing is running. */
    fun stop() {
        strobeJob?.cancel()
        strobeJob = null
        flashCameraId?.let { setTorch(it, enabled = false) }
    }

    private fun setTorch(cameraId: String, enabled: Boolean): Boolean {
        val manager = cameraManager ?: return false
        return try {
            manager.setTorchMode(cameraId, enabled)
            true
        } catch (accessException: CameraAccessException) {
            // Camera in use, disabled by policy, or the device disconnected it.
            Log.w(TAG, "Torch refused (enabled=$enabled)", accessException)
            false
        } catch (cancellation: CancellationException) {
            throw cancellation
        } catch (throwable: Throwable) {
            // setTorchMode also throws IllegalArgumentException for an id that has
            // stopped existing, and some OEM frameworks throw RuntimeException here.
            Log.w(TAG, "Torch unavailable (enabled=$enabled)", throwable)
            false
        }
    }

    private fun findFlashCameraId(): String? {
        val manager = cameraManager ?: return null
        return runCatching {
            manager.cameraIdList.firstOrNull { id ->
                manager.getCameraCharacteristics(id)
                    .get(CameraCharacteristics.FLASH_INFO_AVAILABLE) == true
            }
        }.onFailure { Log.w(TAG, "Could not enumerate cameras", it) }.getOrNull()
    }

    private companion object {
        const val TAG = "TorchController"

        /** Morse time unit; 150 ms reads as deliberate signalling, not a flicker. */
        const val UNIT_MS = 150L

        /**
          * One full **· · · — — — · · ·** cycle as (LED on, duration) steps.
          *
          * Standard Morse timing: a dot is one unit, a dash three, the gap inside a
          * letter one, between letters three, and the trailing word gap seven — long
          * enough that a repeat reads as a repeat rather than one endless burst.
          */
        val SOS_PATTERN: List<Pair<Boolean, Long>> = buildList {
            fun symbol(units: Long) {
                add(true to units * UNIT_MS)
                add(false to UNIT_MS)
            }

            fun letter(units: Long, repeat: Int) {
                repeat(repeat) { symbol(units) }
                // Upgrade the trailing intra-letter gap to a full letter gap.
                add(false to 2 * UNIT_MS)
            }

            letter(units = 1, repeat = 3)  // S: · · ·
            letter(units = 3, repeat = 3)  // O: — — —
            letter(units = 1, repeat = 3)  // S: · · ·
            add(false to 4 * UNIT_MS)      // → 7-unit word gap before repeating
        }
    }
}
