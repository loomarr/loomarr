package tv.loomarr.tv.pairing

import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Exercises the pairing client against a mock server whose responses mirror what the real backend
 * was observed to return — 428 while pending, 404 once dead, 200 with a token on success.
 */
class PairingClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: PairingClient

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        client = PairingClient(server.url("/").toString().trimEnd('/'))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun `start returns the code to display and the poll interval`() =
        runTest {
            server.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .setBody(
                        """{"deviceCode":"secret-abc","userCode":"BCDF-GHJK",
                       "expiresAt":"2026-08-18T01:15:49Z","interval":5}""",
                    ),
            )

            val pairing = client.start("Living Room Shield")

            assertEquals("secret-abc", pairing.deviceCode)
            assertEquals("BCDF-GHJK", pairing.userCode)
            assertEquals(5, pairing.intervalSeconds)

            val sent = server.takeRequest()
            assertEquals("/v1/auth/device/start", sent.path)
            assertTrue(sent.body.readUtf8().contains("Living Room Shield"))
        }

    /** A server that omits the hint must not stop the device pairing. */
    @Test
    fun `start falls back to a default interval`() =
        runTest {
            server.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .setBody("""{"deviceCode":"secret-abc","userCode":"BCDF-GHJK"}"""),
            )

            assertEquals(5, client.start("TV").intervalSeconds)
        }

    /**
     * ⚠ The distinction the whole flow depends on: 428 means WAIT, and must not be reported as a
     * failure. Treating it as one would make the TV discard a perfectly good code every few
     * seconds and show a new one, so nobody could ever finish typing.
     */
    @Test
    fun `a pending pairing reports Pending, not an error`() =
        runTest {
            server.enqueue(MockResponse().setResponseCode(428).setBody("""{"title":"Precondition Required"}"""))

            assertEquals(PollResult.Pending, client.poll("secret-abc"))
        }

    @Test
    fun `an approved pairing yields the durable token`() =
        runTest {
            server.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .setBody("""{"token":"tok-123","deviceName":"Living Room Shield"}"""),
            )

            val result = client.poll("secret-abc")

            assertTrue(result is PollResult.Paired)
            result as PollResult.Paired
            assertEquals("tok-123", result.token)
            assertEquals("Living Room Shield", result.deviceName)
        }

    /** Expired, consumed, and wrong all arrive as 404 and all mean "start over". */
    @Test
    fun `a dead code reports Expired so the device starts over`() =
        runTest {
            server.enqueue(MockResponse().setResponseCode(404).setBody("""{"title":"Pairing not found"}"""))

            assertEquals(PollResult.Expired, client.poll("secret-abc"))
        }

    /** An unexpected status is a real failure and must not masquerade as "keep waiting". */
    @Test(expected = java.io.IOException::class)
    fun `an unexpected status raises`() =
        runTest {
            server.enqueue(MockResponse().setResponseCode(500))

            client.poll("secret-abc")
        }
}
