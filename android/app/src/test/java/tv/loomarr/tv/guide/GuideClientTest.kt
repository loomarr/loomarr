package tv.loomarr.tv.guide

import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant

/**
 * Exercises the guide parser against payloads shaped like the server's own DTO.
 *
 * The field names here were read from `internal/api/guide.go`, not recalled — an unknown JSON key
 * is dropped rather than rejected, so a typo yields an empty grid with nothing in any log.
 */
class GuideClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: GuideClient

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        client = GuideClient(server.url("/").toString().trimEnd('/'), "tok")
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun respond(body: String) {
        server.enqueue(MockResponse().setResponseCode(200).setBody(body))
    }

    @Test
    fun `parses a channel timeline`() =
        runTest {
            respond(
                """
                {"fromMs":1000,"toMs":14400000,"channels":[
                  {"channelId":"ch-1","name":"90s Cartoons","number":12,"status":"live","pendingCount":0,
                   "airings":[
                     {"kind":"program","title":"Wakko's Wish","series":"Animaniacs","season":2,"episode":4,
                      "startMs":1000,"stopMs":1800000}
                   ]}
                ]}
                """.trimIndent(),
            )

            val window = client.window()

            assertEquals(1, window.channels.size)
            val channel = window.channels.first()
            assertEquals("90s Cartoons", channel.name)
            assertEquals(12, channel.number)
            val airing = channel.airings.first()
            assertEquals("Animaniacs", airing.series)
            assertEquals("S2E4", airing.episodeLabel)
            // An episode leads with its SERIES — that is what a viewer scans a guide for.
            assertEquals("Animaniacs", airing.heading)
        }

    /** A film has no series, and "" must not become a series named nothing. */
    @Test
    fun `a film falls back to its own title`() =
        runTest {
            respond(
                """
                {"fromMs":0,"toMs":3600000,"channels":[
                  {"channelId":"ch-2","name":"Horror","number":14,"status":"live","pendingCount":0,
                   "airings":[{"kind":"program","title":"The Thing","startMs":0,"stopMs":6300000}]}
                ]}
                """.trimIndent(),
            )

            val airing = client
                .window()
                .channels
                .first()
                .airings
                .first()

            assertNull(airing.series)
            assertEquals("The Thing", airing.heading)
            assertEquals("", airing.episodeLabel)
        }

    /**
     * ⚠ The property the grid must not get wrong. A nominal block's times are a DISPLAY ESTIMATE,
     * and the server flags it precisely so a client does not present them as scheduled airtime.
     */
    @Test
    fun `carries the nominal flag and its provenance`() =
        runTest {
            respond(
                """
                {"fromMs":0,"toMs":3600000,"channels":[
                  {"channelId":"ch-3","name":"Sci-Fi","number":13,"status":"building","pendingCount":1,
                   "airings":[{"kind":"pending","title":"Solaris","startMs":0,"stopMs":1800000,
                               "nominal":true,"provenance":"acquiring · 62% · 8m left"}]}
                ]}
                """.trimIndent(),
            )

            val airing = client
                .window()
                .channels
                .first()
                .airings
                .first()

            assertTrue(airing.nominal)
            assertEquals("acquiring · 62% · 8m left", airing.provenance)
        }

    /**
     * ⚠ The window the SERVER served, not the one requested. The grid lays blocks out against these
     * bounds, so taking the request instead would draw everything at the wrong offset whenever the
     * server clamped the range.
     */
    @Test
    fun `reports the served window rather than the requested one`() =
        runTest {
            respond("""{"fromMs":500,"toMs":900,"channels":[]}""")

            val window = client.window(fromMs = 0, toMs = 99_999)

            assertEquals(500, window.fromMs)
            assertEquals(900, window.toMs)
            assertEquals(400, window.durationMs)
        }

    @Test
    fun `sends the window as query parameters`() =
        runTest {
            respond("""{"fromMs":0,"toMs":1,"channels":[]}""")

            client.window(fromMs = 111, toMs = 222)

            val path = server.takeRequest().path.orEmpty()
            assertTrue("missing from= in $path", path.contains("from=111"))
            assertTrue("missing to= in $path", path.contains("to=222"))
        }

    @Test
    fun `reads server time from the cheap public health response`() =
        runTest {
            server.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .setHeader("Date", "Sat, 22 Aug 2026 15:30:00 GMT")
                    .setBody("{\"status\":\"ok\"}"),
            )

            assertEquals(Instant.parse("2026-08-22T15:30:00Z").toEpochMilli(), client.serverNowMs())

            val request = server.takeRequest()
            assertEquals("/v1/healthz", request.path)
            assertNull(request.getHeader("Authorization"))
        }

    /** `airingAt` is what the now-playing overlay will read; a gap must be a gap, not the next block. */
    @Test
    fun `finds what is on at an instant, and nothing in a gap`() =
        runTest {
            respond(
                """
                {"fromMs":0,"toMs":3600000,"channels":[
                  {"channelId":"ch-4","name":"Comedy","number":15,"status":"live","pendingCount":0,
                   "airings":[{"kind":"program","title":"Seinfeld","startMs":1000,"stopMs":2000}]}
                ]}
                """.trimIndent(),
            )

            val channel = client.window().channels.first()

            assertEquals("Seinfeld", channel.airingAt(1500)?.title)
            // stopMs is EXCLUSIVE, per the server's own doc comment.
            assertNull(channel.airingAt(2000))
            assertNull(channel.airingAt(500))
        }
}
