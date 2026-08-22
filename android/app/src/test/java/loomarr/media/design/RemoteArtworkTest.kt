package loomarr.media.design

import android.graphics.Bitmap
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okio.Buffer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.GraphicsMode
import java.io.ByteArrayOutputStream

@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class RemoteArtworkTest {
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
    fun `member artwork carries the paired authorization`() =
        runTest {
            val bitmap = Bitmap.createBitmap(2, 1, Bitmap.Config.ARGB_8888)
            val bytes = ByteArrayOutputStream()
                .also {
                    bitmap.compress(
                        Bitmap.CompressFormat.PNG,
                        100,
                        it,
                    )
                }.toByteArray()
            server.enqueue(MockResponse().setResponseCode(200).setBody(Buffer().write(bytes)))

            val loaded =
                ArtworkLoader().load(
                    server.url("/v1/images/hash/w300.jpg").toString(),
                    "Bearer paired-device-token",
                )

            assertNotNull(loaded)
            assertEquals("Bearer paired-device-token", server.takeRequest().getHeader("Authorization"))
        }
}
