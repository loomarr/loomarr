package tv.loomarr.tv.guide

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

class GuideWindowLoaderTest {
    @Test
    fun `opens two readable hours around the server's now without reading device time`() =
        runTest {
            val serverNow = 9_000_000L
            val requests = mutableListOf<Pair<Long?, Long?>>()
            val visible =
                GuideWindow(
                    fromMs = serverNow - 30 * 60 * 1000L,
                    toMs = serverNow + 90 * 60 * 1000L,
                    channels = emptyList(),
                )

            val loaded =
                loadGuideGridWindow(
                    serverNowMs = { serverNow },
                    window = { fromMs, toMs ->
                        requests += fromMs to toMs
                        visible
                    },
                )

            assertEquals(
                listOf(
                    serverNow - 30 * 60 * 1000L to serverNow + 90 * 60 * 1000L,
                ),
                requests,
            )
            assertEquals(serverNow, loaded.nowMs)
            assertEquals(visible, loaded.window)
        }

    @Test
    fun `keeps the server-clamped visible window`() =
        runTest {
            val serverNow = 9_000_000L
            val clamped =
                GuideWindow(
                    fromMs = serverNow - 10 * 60 * 1000L,
                    toMs = serverNow + 210 * 60 * 1000L,
                    channels = emptyList(),
                )
            val loaded =
                loadGuideGridWindow(
                    serverNowMs = { serverNow },
                    window = { _, _ -> clamped },
                )

            assertEquals(serverNow, loaded.nowMs)
            assertEquals(clamped, loaded.window)
        }
}
