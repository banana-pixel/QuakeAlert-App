package id.web.quakealert.domain

/**
 * One dialable emergency number and what answers it.
 *
 * @param label who picks up ("Police", "Fire"), not a sentence — it is rendered as
 *   the row title beside the digits.
 * @param number dialled verbatim. Short codes only: these are national service
 *   numbers, so no country prefix is ever added, and adding one would break them.
 */
data class EmergencyNumber(val label: String, val number: String)

/**
 * Emergency numbers for wherever the device actually is.
 *
 * The app is not Indonesia-only, and a hardcoded 112/113/118 list shown to someone in
 * Tokyo is worse than no list: it looks authoritative. So the table is keyed by
 * ISO-3166-1 alpha-2 and [UNIVERSAL_NUMBER] leads every result.
 *
 * **Why 112 is always first.** It is the GSM-standard emergency number: a handset is
 * required to route it to the local emergency service, it works with no SIM, with a
 * locked keypad and on a foreign network. That makes it the only correct answer when
 * the country is unknown, and a safe first answer when it is known — so an unresolved
 * country shows 112 alone rather than a guess.
 *
 * The table is bundled rather than fetched. This screen is most needed when the
 * network is gone, which is the same moment a lookup would fail.
 *
 * Labels stay in Kotlin because the *number* is data, not translatable copy; only the
 * words around them belong in `strings.xml`.
 */
object EmergencyContacts {

    /** Works on any GSM network, with or without a SIM. Always offered first. */
    const val UNIVERSAL_NUMBER: String = "112"

    /**
     * [UNIVERSAL_NUMBER] followed by whatever the country adds to it.
     *
     * @param countryIso ISO-3166-1 alpha-2, case-insensitive, or null when the device
     *   could not say — from a SIM-less phone, or an unregistered network.
     */
    fun forCountry(countryIso: String?): List<EmergencyNumber> {
        val universal = EmergencyNumber(label = "Emergency (any network)", number = UNIVERSAL_NUMBER)
        val local = countryIso
            ?.trim()
            ?.takeIf { it.length == 2 }
            ?.uppercase()
            ?.let { TABLE[it] }
            .orEmpty()
        // A country whose own emergency number *is* 112 must not print it twice.
        return listOf(universal) + local.filterNot { it.number == UNIVERSAL_NUMBER }
    }

    /**
     * Country-specific numbers, seeded with the app's own market and the
     * earthquake-prone countries it is most likely to travel to.
     *
     * An absent country is not a gap to fill with a neighbour's numbers: it falls
     * back to 112 alone, which routes correctly everywhere GSM does.
     */
    private val TABLE: Map<String, List<EmergencyNumber>> = mapOf(
        "ID" to listOf(
            EmergencyNumber("Police", "110"),
            EmergencyNumber("Fire", "113"),
            EmergencyNumber("Ambulance", "118"),
            EmergencyNumber("Search and rescue (Basarnas)", "115")
        ),
        "US" to listOf(EmergencyNumber("Emergency", "911")),
        "CA" to listOf(EmergencyNumber("Emergency", "911")),
        "MX" to listOf(EmergencyNumber("Emergency", "911")),
        "PH" to listOf(EmergencyNumber("Emergency", "911")),
        "GB" to listOf(EmergencyNumber("Emergency", "999")),
        "AU" to listOf(EmergencyNumber("Emergency", "000")),
        "NZ" to listOf(EmergencyNumber("Emergency", "111")),
        "MY" to listOf(EmergencyNumber("Emergency", "999")),
        "SG" to listOf(
            EmergencyNumber("Police", "999"),
            EmergencyNumber("Fire and ambulance", "995")
        ),
        "JP" to listOf(
            EmergencyNumber("Police", "110"),
            EmergencyNumber("Fire and ambulance", "119")
        ),
        "KR" to listOf(EmergencyNumber("Fire and ambulance", "119")),
        "TW" to listOf(
            EmergencyNumber("Police", "110"),
            EmergencyNumber("Fire and ambulance", "119")
        ),
        "CN" to listOf(
            EmergencyNumber("Police", "110"),
            EmergencyNumber("Fire", "119"),
            EmergencyNumber("Ambulance", "120")
        ),
        "IN" to listOf(
            EmergencyNumber("Police", "100"),
            EmergencyNumber("Fire", "101"),
            EmergencyNumber("Ambulance", "102")
        ),
        "CL" to listOf(
            EmergencyNumber("Police", "133"),
            EmergencyNumber("Fire", "132"),
            EmergencyNumber("Ambulance", "131")
        ),
        "NP" to listOf(
            EmergencyNumber("Police", "100"),
            EmergencyNumber("Ambulance", "102")
        ),
        "TR" to listOf(EmergencyNumber("Emergency", "112"))
    )
}
