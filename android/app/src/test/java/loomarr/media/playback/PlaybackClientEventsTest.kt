package loomarr.media.playback

import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

class PlaybackClientEventsTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun `authenticated stream exposes connection and channel invalidation only`() =
        runTest {
            server.enqueue(
                MockResponse()
                    .setHeader("Content-Type", "text/event-stream")
                    .setBody(
                        ": connected\n\n" +
                            "event: title\ndata: {\"key\":\"series:1\",\"state\":\"available\"}\n\n" +
                            "event: channel\ndata: {\"channelId\":\"ch-2\",\"status\":\"live\"}\n\n",
                    ),
            )
            val client = PlaybackClient(server.url("/").toString().trimEnd('/'), "paired-token")

            val events = client.channelEvents().take(2).toList()

            assertEquals(
                listOf(ChannelStreamEvent.Connected, ChannelStreamEvent.ChannelChanged),
                events,
            )
            val request = server.takeRequest()
            assertEquals("/v1/events", request.path)
            assertEquals("Bearer paired-token", request.getHeader("Authorization"))
            assertEquals("text/event-stream", request.getHeader("Accept"))
        }
}
