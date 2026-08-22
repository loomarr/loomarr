package loomarr.media.playback

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Test
import java.io.IOException

@OptIn(ExperimentalCoroutinesApi::class)
class ChannelCatalogTest {
    @Test
    fun `channel event reconciles additions through the authoritative list`() =
        runTest {
            val source = FakeCatalogClient(listOf(channel("one", 1)))
            val catalog = catalog(source)

            catalog.start()
            runCurrent()
            assertEquals(listOf("one"), catalog.ready().channels.map { it.id })

            source.lineup = listOf(channel("one", 1), channel("two", 2))
            source.events.emit(ChannelStreamEvent.ChannelChanged)
            runCurrent()

            assertEquals(listOf("one", "two"), catalog.ready().channels.map { it.id })
            catalog.close()
        }

    @Test
    fun `failed refresh retains the last complete catalog`() =
        runTest {
            val source = FakeCatalogClient(listOf(channel("one", 1)))
            val catalog = catalog(source)
            catalog.start()
            runCurrent()
            val firstRevision = catalog.ready().revision

            source.failure = IOException("network down")
            catalog.requestRefresh()
            runCurrent()

            val stale = catalog.ready()
            assertEquals(listOf("one"), stale.channels.map { it.id })
            assertEquals(firstRevision, stale.revision)
            assertEquals("network down", stale.refreshError)
            catalog.close()
        }

    @Test
    fun `reconnect performs a full reconciliation without a channel frame`() =
        runTest {
            val source = ReconnectingCatalogClient()
            val catalog =
                ChannelCatalog(
                    client = source,
                    scope = CoroutineScope(coroutineContext + SupervisorJob()),
                    coalesceMillis = 0,
                    initialReconnectMillis = 10,
                    maximumReconnectMillis = 10,
                )

            catalog.start()
            runCurrent()
            source.lineup = listOf(channel("one", 1), channel("two", 2))
            advanceTimeBy(10)
            runCurrent()

            assertEquals(listOf("one", "two"), catalog.ready().channels.map { it.id })
            assertEquals(2, source.connections)
            catalog.close()
        }

    @Test
    fun `safety reconciliation bounds staleness when a frame is dropped`() =
        runTest {
            val source = FakeCatalogClient(listOf(channel("one", 1)))
            val catalog =
                ChannelCatalog(
                    client = source,
                    scope = CoroutineScope(coroutineContext + SupervisorJob()),
                    coalesceMillis = 0,
                    safetyRefreshMillis = 50,
                )
            catalog.start()
            runCurrent()

            source.lineup = listOf(channel("one", 1), channel("two", 2))
            // Deliberately do not emit: this is the lossy-bus recovery path.
            advanceTimeBy(50)
            runCurrent()

            assertEquals(listOf("one", "two"), catalog.ready().channels.map { it.id })
            catalog.close()
        }

    @Test
    fun `foreground entry reconciles changes missed while stopped`() =
        runTest {
            val source = FakeCatalogClient(listOf(channel("one", 1)))
            val catalog = catalog(source)
            catalog.start()
            runCurrent()
            catalog.stop()

            source.lineup = listOf(channel("one", 1), channel("two", 2))
            source.events.emit(ChannelStreamEvent.ChannelChanged)
            runCurrent()
            assertEquals(listOf("one"), catalog.ready().channels.map { it.id })

            catalog.start()
            runCurrent()
            assertEquals(listOf("one", "two"), catalog.ready().channels.map { it.id })
            catalog.close()
        }

    @Test
    fun `burst of channel frames is coalesced into one read`() =
        runTest {
            val source = FakeCatalogClient(listOf(channel("one", 1)))
            val catalog =
                ChannelCatalog(
                    client = source,
                    scope = CoroutineScope(coroutineContext + SupervisorJob()),
                    coalesceMillis = 50,
                )
            catalog.start()
            advanceTimeBy(50)
            runCurrent()
            val initialReads = source.channelReads

            repeat(8) { source.events.emit(ChannelStreamEvent.ChannelChanged) }
            runCurrent()
            advanceTimeBy(50)
            runCurrent()

            assertEquals(initialReads + 1, source.channelReads)
            catalog.close()
        }

    private fun kotlinx.coroutines.test.TestScope.catalog(source: ChannelCatalogClient) =
        ChannelCatalog(
            client = source,
            scope = CoroutineScope(coroutineContext + SupervisorJob()),
            coalesceMillis = 0,
            initialReconnectMillis = 10,
            maximumReconnectMillis = 10,
        )

    private fun ChannelCatalog.ready(): ChannelCatalogState.Ready {
        val ready = state.value as? ChannelCatalogState.Ready
        assertNotNull("catalog was ${state.value}", ready)
        return requireNotNull(ready)
    }

    private class FakeCatalogClient(
        var lineup: List<Channel>,
    ) : ChannelCatalogClient {
        val events = MutableSharedFlow<ChannelStreamEvent>(extraBufferCapacity = 1)
        var failure: Exception? = null
        var channelReads = 0

        override suspend fun channels(): List<Channel> {
            channelReads++
            failure?.let { throw it }
            return lineup
        }

        override fun channelEvents(): Flow<ChannelStreamEvent> =
            flow {
                emit(ChannelStreamEvent.Connected)
                emitAll(events)
            }
    }

    private class ReconnectingCatalogClient : ChannelCatalogClient {
        var lineup = listOf(channel("one", 1))
        var connections = 0

        override suspend fun channels(): List<Channel> = lineup

        override fun channelEvents(): Flow<ChannelStreamEvent> =
            flow {
                connections++
                emit(ChannelStreamEvent.Connected)
                if (connections == 1) throw IOException("proxy restarted")
                awaitCancellation()
            }
    }

    private companion object {
        fun channel(
            id: String,
            number: Int,
        ) = Channel(
            id = id,
            name = "Channel $number",
            number = number,
            inAppPlayable = true,
        )
    }
}
