package id.web.quakealert.ui.common

import id.web.quakealert.data.network.ApiException
import java.io.IOException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import javax.net.ssl.SSLException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Test

/**
 * Guards the one rule the whole mapper exists for: the failure's *kind* picks the
 * copy, and the server's own message never becomes it.
 *
 * The server speaks Indonesian to operators ("range_km butuh latitude & longitude"),
 * and OkHttp speaks socket states. Both used to reach the screen verbatim, so these
 * tests use exactly that kind of text as the failure message and assert it does not
 * come back out.
 */
class ErrorCopyTest {

    private val SERVER_TEXT = "range_km butuh latitude & longitude"

    @Test
    fun `transport failures read as offline and offer a retry`() {
        val failures = listOf(
            UnknownHostException("api.quakealert.web.id"),
            SocketTimeoutException("timeout"),
            SSLException("handshake"),
            IOException("unexpected end of stream")
        )
        failures.forEach { failure ->
            val copy = errorCopy(failure)
            assertEquals("You are offline", copy.title)
            assertEquals(ErrorAction.RETRY, copy.action)
        }
    }

    @Test
    fun `an expired identity names the sign-in and offers a retry`() {
        val copy = errorCopy(ApiException(401, "UNAUTHENTICATED", "token expired"))

        assertEquals("Sign-in expired", copy.title)
        assertEquals(ErrorAction.RETRY, copy.action)
    }

    @Test
    fun `a rejected request does not offer a retry that cannot help`() {
        val copy = errorCopy(ApiException(400, "INVALID_ARGUMENT", SERVER_TEXT))

        assertEquals("That request was not accepted", copy.title)
        assertEquals(ErrorAction.NONE, copy.action)
    }

    @Test
    fun `a rejected request with a narrowing filter offers the filter reset`() {
        val copy = errorCopy(
            ApiException(400, "INVALID_ARGUMENT", SERVER_TEXT),
            isNarrowed = true
        )

        assertEquals(ErrorAction.RESET_FILTERS, copy.action)
    }

    @Test
    fun `rate limiting is its own copy, not a generic rejection`() {
        val copy = errorCopy(ApiException(429, "RATE_LIMITED", "too many"))

        assertEquals("Too many requests", copy.title)
        assertEquals(ErrorAction.RETRY, copy.action)
        assertNotEquals("That request was not accepted", copy.title)
    }

    @Test
    fun `a server fault falls back to the generic copy`() {
        val copy = errorCopy(ApiException(500, "INTERNAL", "pq: connection refused"))

        assertEquals(GenericErrorCopy, copy)
        assertEquals(ErrorAction.RETRY, copy.action)
    }

    @Test
    fun `no branch lets the raw failure text reach the screen`() {
        val failures = listOf(
            UnknownHostException(SERVER_TEXT),
            SocketTimeoutException(SERVER_TEXT),
            IOException(SERVER_TEXT),
            IllegalStateException(SERVER_TEXT),
            ApiException(400, "INVALID_ARGUMENT", SERVER_TEXT),
            ApiException(401, "UNAUTHENTICATED", SERVER_TEXT),
            ApiException(429, "RATE_LIMITED", SERVER_TEXT),
            ApiException(500, "INTERNAL", SERVER_TEXT)
        )
        failures.forEach { failure ->
            listOf(false, true).forEach { narrowed ->
                val copy = errorCopy(failure, isNarrowed = narrowed)
                assertFalse(copy.title.contains(SERVER_TEXT))
                assertFalse(copy.message.contains(SERVER_TEXT))
                // Nor the exception's own class name, which is the other thing a
                // `throwable.toString()` fallback would have leaked.
                assertFalse(copy.message.contains(failure.javaClass.simpleName))
            }
        }
    }
}
