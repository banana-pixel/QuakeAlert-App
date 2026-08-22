package id.web.quakealert.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Guards the one thing this table must never do: hand someone a number that rings
 * nowhere they are.
 *
 * The failure it exists to catch is a confident wrong answer — an Indonesian list
 * shown to a phone attached to a Japanese network, or a guessed national number for a
 * country the table has never heard of. 112 is the safe answer in both cases, so it
 * leads every result and stands alone when nothing else is known.
 */
class EmergencyContactsTest {

    @Test
    fun `an unknown country gets the universal number and nothing invented`() {
        listOf(null, "", "  ", "XX", "ZZZ", "I").forEach { iso ->
            val numbers = EmergencyContacts.forCountry(iso)
            assertEquals("failed for $iso", 1, numbers.size)
            assertEquals(EmergencyContacts.UNIVERSAL_NUMBER, numbers.single().number)
        }
    }

    @Test
    fun `a known country keeps 112 first and adds its own numbers`() {
        val numbers = EmergencyContacts.forCountry("ID")

        assertEquals(EmergencyContacts.UNIVERSAL_NUMBER, numbers.first().number)
        assertEquals(listOf("112", "110", "113", "118", "115"), numbers.map { it.number })
    }

    @Test
    fun `the country code is read case-insensitively and untrimmed`() {
        assertEquals(
            EmergencyContacts.forCountry("ID"),
            EmergencyContacts.forCountry(" id ")
        )
    }

    @Test
    fun `a country whose own number is 112 does not print it twice`() {
        // Turkey consolidated everything onto 112; the table carries it, and the
        // universal entry must absorb it rather than repeat it.
        val numbers = EmergencyContacts.forCountry("TR")

        assertEquals(1, numbers.count { it.number == EmergencyContacts.UNIVERSAL_NUMBER })
        assertEquals(1, numbers.size)
    }

    @Test
    fun `every entry is a bare dialable short code`() {
        listOf("ID", "US", "JP", "GB", "AU", "NZ", "IN", "SG", "MY", "CN", "TW", "CL")
            .flatMap { EmergencyContacts.forCountry(it) }
            .forEach { entry ->
                // No country prefix, no punctuation, no spaces: these are national
                // service codes and a "+" in front of one breaks it.
                assertTrue(entry.number, entry.number.all { it.isDigit() })
                assertTrue(entry.number, entry.number.length in 3..4)
                assertTrue(entry.label, entry.label.isNotBlank())
            }
    }
}
