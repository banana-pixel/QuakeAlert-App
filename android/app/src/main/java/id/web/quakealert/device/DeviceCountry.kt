package id.web.quakealert.device

import android.content.Context
import android.telephony.TelephonyManager

/**
 * Which country's emergency services the device can currently reach.
 *
 * Network before SIM, deliberately: what matters is the service that would answer,
 * and that belongs to the network the phone is attached to. An Indonesian SIM roaming
 * in Japan should offer 110/119, not 113 — the reverse order would hand out numbers
 * that ring nowhere.
 *
 * No permission is involved: both reads are ungated, unlike anything based on an
 * actual position fix. That is also why this is preferred over reverse-geocoding the
 * last known coordinates, which would need location permission and a network.
 *
 * Returns null rather than a locale guess when neither is available — a SIM-less
 * tablet on Wi-Fi has no country, and `Locale` reports the language the user reads,
 * not the country they are standing in.
 */
object DeviceCountry {

    /** ISO-3166-1 alpha-2, uppercased, or null when the device cannot say. */
    fun resolve(context: Context): String? {
        val telephony = context.getSystemService(Context.TELEPHONY_SERVICE) as? TelephonyManager
            ?: return null
        return telephony.networkCountryIso.orNullIso() ?: telephony.simCountryIso.orNullIso()
    }

    private fun String?.orNullIso(): String? =
        this?.trim()?.takeIf { it.length == 2 }?.uppercase()
}
