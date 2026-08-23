package loomarr.media.version

import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Before
import org.junit.Test

class VersionIdentityTest {
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
    fun `label keeps client and server identities separate`() {
        assertEquals(
            "Loomarr TV 0.1.0-dev-debug · Server v0.9.3 (modified)",
            VersionIdentity("0.1.0-dev-debug", "v0.9.3 (modified)").label,
        )
    }

    @Test
    fun `client reads server truth and preserves dirty build semantics`() =
        runTest {
            server.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .setBody("""{"version":"v0.9.3","dirty":true,"ready":true}"""),
            )

            val version = ServerVersionClient(server.url("/").toString()).fetch()

            assertEquals("v0.9.3 (modified)", version.displayName)
            assertEquals("/v1/system/version", server.takeRequest().path)
        }

    @Test
    fun `clean response stays unqualified`() =
        runTest {
            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"version":"dev","ready":true}"""))

            val version = ServerVersionClient(server.url("/").toString()).fetch()

            assertEquals("dev", version.displayName)
            assertFalse(version.dirty)
        }
}
