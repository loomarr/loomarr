package loomarr.media.diagnostics

import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
class ClientDiagnosticsTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun `sender uses the authenticated closed batch`() =
        runTest {
            server.enqueue(MockResponse().setResponseCode(202).setBody("{\"accepted\":1}"))
            HttpClientDiagnosticSender(
                server.url("/").toString(),
                "paired-token",
                "0.2.0",
                "shield_tv",
            ).send(listOf(observation(ClientEvent.PlayerReady)))

            val request = server.takeRequest()
            assertEquals("/v1/diagnostics/client-events", request.path)
            assertEquals("Bearer paired-token", request.getHeader("Authorization"))
            val body = request.body.readUtf8()
            assertTrue(body.contains("\"source\":\"android_tv\""))
            assertTrue(body.contains("\"event\":\"player.ready\""))
            assertFalse(body.contains("paired-token"))
        }

    @Test
    fun `reporting batches asynchronously`() =
        runTest {
            val sent = mutableListOf<List<ClientObservation>>()
            val reporter = ClientDiagnosticsReporter({ sent += it }, this)
            reporter.record(observation(ClientEvent.PlayerAttached))
            assertTrue(sent.isEmpty())

            advanceTimeBy(2_001)
            assertEquals(1, sent.size)
            assertEquals(ClientEvent.PlayerAttached, sent.single().single().event)
        }

    private fun TestScope.observation(event: ClientEvent) =
        ClientObservation(
            event = event,
            occurredAt = testScheduler.currentTime + 1,
            playbackSessionId = "session_1",
            channelId = "channel_1",
        )
}
