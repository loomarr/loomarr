package tv.loomarr.tv.playback

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.delay
import tv.loomarr.tv.design.Body
import tv.loomarr.tv.design.DeadAir
import tv.loomarr.tv.design.ErrorText
import tv.loomarr.tv.design.LoomarrTokens
import tv.loomarr.tv.design.MonoData
import tv.loomarr.tv.design.OverscanMargin
import tv.loomarr.tv.design.SectionHeading
import tv.loomarr.tv.design.TuningText
import tv.loomarr.tv.guide.Airing
import tv.loomarr.tv.guide.ChannelTimeline
import tv.loomarr.tv.guide.GuideUiState
import tv.loomarr.tv.guide.GuideViewModel
import tv.loomarr.tv.guide.rememberServerNow
import tv.loomarr.tv.navigation.WatchingRemoteAction
import tv.loomarr.tv.navigation.channelIndexForNumber
import tv.loomarr.tv.navigation.channelSections
import tv.loomarr.tv.navigation.watchingRemoteAction

/** Full-screen playback plus the mock's Watching and Surf remote states. */
@Composable
fun WatchScreen(
    model: WatchViewModel,
    guideModel: GuideViewModel,
    showingSurf: Boolean = false,
    onOpenGuide: () -> Unit = {},
    onOpenSurf: () -> Unit = {},
    onCloseSurf: () -> Unit = {},
) {
    val state by model.state.collectAsStateWithLifecycle()
    val guide by guideModel.state.collectAsStateWithLifecycle()
    val guideAnchor = (guide as? GuideUiState.Ready)?.nowMs ?: 0L
    val liveGuideNow = rememberServerNow(guideAnchor)
    val liveGuide =
        (guide as? GuideUiState.Ready)?.copy(nowMs = liveGuideNow)
            ?: guide
    val focus = remember { FocusRequester() }
    var bannerNonce by remember { mutableIntStateOf(0) }
    var numberEntry by remember { mutableStateOf("") }

    LaunchedEffect(showingSurf) {
        if (!showingSurf) {
            withFrameNanos { }
            focus.requestFocus()
        }
    }
    LaunchedEffect(numberEntry) {
        if (numberEntry.isEmpty()) return@LaunchedEffect
        delay(NUMBER_ENTRY_MS)
        model.tuneChannelNumber(numberEntry)
        numberEntry = ""
    }

    Box(
        modifier =
            Modifier
                .fillMaxSize()
                .background(LoomarrTokens.Color.Static950)
                .focusRequester(focus)
                .focusable()
                .onKeyEvent { event ->
                    if (showingSurf || event.type != KeyEventType.KeyDown) return@onKeyEvent false
                    remoteDigit(event.key)?.let { digit ->
                        numberEntry = (numberEntry + digit).takeLast(MAX_CHANNEL_DIGITS)
                        bannerNonce++
                        return@onKeyEvent true
                    }
                    bannerNonce++
                    when (watchingRemoteAction(event.key)) {
                        WatchingRemoteAction.ChannelUp -> {
                            model.channelUp()
                            true
                        }
                        WatchingRemoteAction.ChannelDown -> {
                            model.channelDown()
                            true
                        }
                        WatchingRemoteAction.OpenGuide -> {
                            if (numberEntry.isNotEmpty()) {
                                model.tuneChannelNumber(numberEntry)
                                numberEntry = ""
                            } else {
                                onOpenGuide()
                            }
                            true
                        }
                        WatchingRemoteAction.OpenSurf -> {
                            onOpenSurf()
                            true
                        }
                        null -> false
                    }
                },
    ) {
        when (val current = state) {
            is WatchUiState.Loading ->
                TuningText("Tuning in…", modifier = Modifier.align(Alignment.Center))
            is WatchUiState.Failed ->
                ErrorText(current.message, modifier = Modifier.align(Alignment.Center))
            is WatchUiState.DeadAir ->
                DeadAir(
                    title = "Dead air",
                    description = "No channels are scheduled yet. Create one in Loomarr and it will appear here.",
                    modifier = Modifier.align(Alignment.Center),
                )
            is WatchUiState.Ready -> {
                val channel = current.channels[current.selected]
                if (current.playUrl != null) {
                    PlayerScreen(playUrl = current.playUrl)
                } else {
                    TuningText("Tuning ${channel.name}…", modifier = Modifier.align(Alignment.Center))
                }

                WatchingChrome(
                    channel = channel,
                    guide = liveGuide,
                    playing = current.playUrl != null,
                    numberEntry = numberEntry,
                    numberEntryChannelName =
                        channelIndexForNumber(current.channels, numberEntry)
                            ?.let(current.channels::get)
                            ?.name,
                    visibleNonce = bannerNonce + current.selected,
                    showProgrammeBar = !showingSurf,
                    modifier = Modifier.watchingChromeContainer(),
                )

                if (showingSurf) {
                    SurfRail(
                        state = current,
                        guide = liveGuide,
                        onTune = {
                            model.tuneChannelId(it.id)
                            onCloseSurf()
                        },
                        onCancel = onCloseSurf,
                    )
                }
            }
        }
    }
}

/** The Watching chrome: Channel identity, programme detail, progress, next, and remote hints. */
@Composable
internal fun WatchingChrome(
    channel: Channel,
    guide: GuideUiState,
    numberEntry: String,
    visibleNonce: Int,
    modifier: Modifier = Modifier,
    numberEntryChannelName: String? = null,
    playing: Boolean = true,
    showProgrammeBar: Boolean = true,
) {
    val guideInfo = guide.infoFor(channel.id)
    var visible by remember { mutableStateOf(true) }
    LaunchedEffect(visibleNonce) {
        visible = true
        delay(BANNER_VISIBLE_MS)
        visible = false
    }

    Box(modifier = modifier) {
        AnimatedVisibility(
            visible = visible,
            enter = fadeIn(),
            exit = fadeOut(),
            modifier = Modifier.align(Alignment.TopEnd).padding(OverscanMargin),
        ) {
            ChannelPill(channel = channel, playing = playing)
        }
        if (numberEntry.isNotEmpty()) {
            NumberEntry(
                digits = numberEntry,
                channelName = numberEntryChannelName,
                modifier = Modifier.align(Alignment.TopStart).padding(OverscanMargin),
            )
        }
        AnimatedVisibility(
            visible = visible && showProgrammeBar,
            enter = slideInVertically { it } + fadeIn(),
            exit = slideOutVertically { it } + fadeOut(),
            modifier = Modifier.align(Alignment.BottomStart).fillMaxWidth(),
        ) {
            NowPlayingBar(channel = channel, info = guideInfo, modifier = Modifier.fillMaxWidth())
        }
    }
}

@Composable
private fun ChannelPill(
    channel: Channel,
    playing: Boolean,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier =
            modifier
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Md))
                .background(LoomarrTokens.Color.Static950.copy(alpha = 0.72f))
                .border(1.dp, LoomarrTokens.Color.Static700, RoundedCornerShape(LoomarrTokens.Radius.Md))
                .padding(horizontal = LoomarrTokens.Space.S4, vertical = LoomarrTokens.Space.S2),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        MonoData(channel.number.toString().padStart(2, '0'), color = LoomarrTokens.Color.Signal)
        Body(
            channel.name.uppercase(),
            color = LoomarrTokens.Color.Static100,
            fontSize = LoomarrTokens.Type.Sm,
            maxLines = 1,
            modifier = Modifier.padding(start = LoomarrTokens.Space.S3),
        )
        if (playing) {
            Box(
                modifier =
                    Modifier
                        .padding(start = LoomarrTokens.Space.S3)
                        .width(8.dp)
                        .height(8.dp)
                        .clip(RoundedCornerShape(LoomarrTokens.Radius.Lg))
                        .background(LoomarrTokens.Color.Lock),
            )
        }
    }
}

@Composable
private fun NumberEntry(
    digits: String,
    channelName: String?,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier =
            modifier
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Md))
                .background(LoomarrTokens.Color.Static950.copy(alpha = 0.82f))
                .border(
                    1.dp,
                    LoomarrTokens.Color.Signal.copy(alpha = 0.4f),
                    RoundedCornerShape(LoomarrTokens.Radius.Md),
                ).padding(horizontal = LoomarrTokens.Space.S5, vertical = LoomarrTokens.Space.S3),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        androidx.compose.material3.Text(
            text = digits.toCharArray().joinToString(" ") + " _",
            color = LoomarrTokens.Color.Signal,
            fontSize = LoomarrTokens.Type.Xl,
            fontFamily = FontFamily.Monospace,
        )
        channelName?.let {
            Body(it, maxLines = 1, modifier = Modifier.padding(start = LoomarrTokens.Space.S4))
        }
    }
}

@Composable
private fun NowPlayingBar(
    channel: Channel,
    info: ChannelGuideInfo?,
    modifier: Modifier = Modifier,
) {
    val contentInset = OverscanMargin + LoomarrTokens.Space.S4
    Column(
        modifier =
            modifier
                .testTag(NOW_PLAYING_BAR_TAG)
                .fillMaxWidth()
                .background(LoomarrTokens.Color.Static950.copy(alpha = 0.78f))
                .padding(top = LoomarrTokens.Space.S6),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = contentInset),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Body(
                info?.current?.let { current ->
                    current.episodeTitle?.let { "${current.heading} — “$it”" } ?: current.heading
                } ?: channel.name,
                color = LoomarrTokens.Color.Static0,
                fontSize = LoomarrTokens.Type.Lg,
                maxLines = 1,
                modifier = Modifier.weight(1f),
            )
            info?.current?.episodeLabel?.takeIf(String::isNotEmpty)?.let {
                MonoData(
                    it,
                    color = LoomarrTokens.Color.Static400,
                    modifier = Modifier.padding(start = LoomarrTokens.Space.S3),
                )
            }
            info?.current?.year?.takeIf { it > 0 }?.let {
                MonoData(
                    it.toString(),
                    color = LoomarrTokens.Color.Static400,
                    fontSize = LoomarrTokens.Type.Sm,
                    modifier = Modifier.padding(start = LoomarrTokens.Space.S3),
                )
            }
            info?.current?.let {
                MonoData(
                    "${tv.loomarr.tv.guide.clockLabel(it.startMs)}–${tv.loomarr.tv.guide.clockLabel(it.stopMs)}",
                    color = LoomarrTokens.Color.Static400,
                    fontSize = LoomarrTokens.Type.Sm,
                    modifier = Modifier.padding(start = LoomarrTokens.Space.S3),
                )
            }
        }
        Box(
            modifier =
                Modifier
                    .padding(top = LoomarrTokens.Space.S3)
                    .fillMaxWidth()
                    .height(5.dp)
                    .clip(RoundedCornerShape(LoomarrTokens.Radius.Sm))
                    .background(LoomarrTokens.Color.Static800),
        ) {
            Box(
                modifier =
                    Modifier
                        .fillMaxWidth(info?.progress ?: 0f)
                        .fillMaxHeight()
                        .background(LoomarrTokens.Color.Signal),
            )
        }
        Row(
            modifier =
                Modifier
                    .fillMaxWidth()
                    .padding(
                        start = contentInset,
                        top = LoomarrTokens.Space.S3,
                        end = contentInset,
                        bottom = LoomarrTokens.Space.S4,
                    ),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Body(
                info?.next?.let { "Up next · ${tv.loomarr.tv.guide.clockLabel(it.startMs)} — ${it.heading}" }
                    ?: "No later programme in this guide window",
                maxLines = 1,
                modifier = Modifier.weight(1f),
            )
            MonoData(
                "▲▼ tune · ◀ channels · 0–9 jump · OK guide",
                color = LoomarrTokens.Color.Static500,
                fontSize = LoomarrTokens.Type.Xs2,
                maxLines = 1,
                modifier = Modifier.padding(start = LoomarrTokens.Space.S6),
            )
        }
    }
}

internal fun Modifier.watchingChromeContainer(): Modifier = fillMaxSize()

internal const val NOW_PLAYING_BAR_TAG = "now-playing-bar"

/** The mock's grouped Surf rail. It owns focus while open and never tears down the player below. */
@Composable
internal fun SurfRail(
    state: WatchUiState.Ready,
    guide: GuideUiState,
    onTune: (Channel) -> Unit,
    onCancel: () -> Unit,
) {
    val sections = channelSections(state.channels, emptySet(), state.recentChannelIds)
    val rows =
        buildList {
            sections.forEach { section ->
                add(SurfRow.Heading(section.title, section.channels.size))
                if (section.channels.isEmpty()) add(SurfRow.Empty(section.title))
                section.channels.forEach { add(SurfRow.ChannelRow(it)) }
            }
        }
    val selectable = rows.indices.filter { rows[it] is SurfRow.ChannelRow }
    val currentId = state.channels[state.selected].id
    var selected by remember(rows) {
        mutableIntStateOf(
            selectable.firstOrNull { (rows[it] as SurfRow.ChannelRow).channel.id == currentId }
                ?: selectable.firstOrNull()
                ?: 0,
        )
    }
    val focus = remember { FocusRequester() }
    val list = rememberLazyListState()
    LaunchedEffect(Unit) {
        withFrameNanos { }
        focus.requestFocus()
    }
    LaunchedEffect(selected) { list.scrollToItem(selected) }

    Box(
        modifier =
            Modifier
                .fillMaxSize()
                .background(LoomarrTokens.Color.Static950.copy(alpha = 0.46f))
                .focusRequester(focus)
                .focusable()
                .onKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onKeyEvent false
                    val position = selectable.indexOf(selected).coerceAtLeast(0)
                    when (event.key) {
                        Key.DirectionUp -> {
                            selected = selectable[(position - 1).coerceAtLeast(0)]
                            true
                        }
                        Key.DirectionDown -> {
                            selected = selectable[(position + 1).coerceAtMost(selectable.lastIndex)]
                            true
                        }
                        Key.DirectionCenter, Key.Enter -> {
                            (rows.getOrNull(selected) as? SurfRow.ChannelRow)?.channel?.let(onTune)
                            true
                        }
                        Key.Back, Key.Menu -> {
                            onCancel()
                            true
                        }
                        else -> false
                    }
                },
    ) {
        Column(
            modifier =
                Modifier
                    .fillMaxHeight()
                    .width(SURF_RAIL_WIDTH)
                    .background(
                        Brush.horizontalGradient(
                            listOf(
                                LoomarrTokens.Color.Static950.copy(alpha = 0.88f),
                                LoomarrTokens.Color.Static950.copy(alpha = 0.78f),
                            ),
                        ),
                    ).padding(
                        start = OverscanMargin,
                        top = OverscanMargin,
                        end = LoomarrTokens.Space.S4,
                        bottom = OverscanMargin,
                    ),
        ) {
            LazyColumn(
                state = list,
                modifier = Modifier.weight(1f).fillMaxWidth(),
                verticalArrangement = Arrangement.spacedBy(LoomarrTokens.Space.S1),
            ) {
                items(rows.size) { index ->
                    when (val row = rows[index]) {
                        is SurfRow.Heading ->
                            SectionHeading(
                                "${row.title} · ${row.count}",
                                modifier =
                                    Modifier.padding(
                                        top = LoomarrTokens.Space.S3,
                                        bottom = LoomarrTokens.Space.S1,
                                    ),
                            )
                        is SurfRow.Empty ->
                            Body(
                                if (row.section == "Favorites") "No favorites yet" else "No recent channels yet",
                                fontSize = LoomarrTokens.Type.Xs,
                            )
                        is SurfRow.ChannelRow ->
                            SurfChannelRow(
                                channel = row.channel,
                                info = guide.infoFor(row.channel.id),
                                focused = index == selected,
                                watching = row.channel.id == currentId,
                            )
                    }
                }
            }
            MonoData(
                "${selectable.indexOf(selected) + 1} of ${selectable.size} · ▲▼ browse",
                color = LoomarrTokens.Color.Static500,
                fontSize = LoomarrTokens.Type.Xs2,
                maxLines = 1,
                modifier = Modifier.padding(top = LoomarrTokens.Space.S2),
            )
        }
        MonoData(
            "OK tune · BACK cancel",
            color = LoomarrTokens.Color.Static400,
            fontSize = LoomarrTokens.Type.Xs2,
            maxLines = 1,
            modifier =
                Modifier
                    .align(Alignment.BottomEnd)
                    .padding(OverscanMargin)
                    .clip(RoundedCornerShape(LoomarrTokens.Radius.Md))
                    .background(LoomarrTokens.Color.Static950.copy(alpha = 0.78f))
                    .border(
                        1.dp,
                        LoomarrTokens.Color.Static700,
                        RoundedCornerShape(LoomarrTokens.Radius.Md),
                    ).padding(horizontal = LoomarrTokens.Space.S3, vertical = LoomarrTokens.Space.S2),
        )
    }
}

@Composable
private fun SurfChannelRow(
    channel: Channel,
    info: ChannelGuideInfo?,
    focused: Boolean,
    watching: Boolean,
) {
    Column(
        modifier =
            Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(LoomarrTokens.Radius.Lg))
                .background(if (focused) LoomarrTokens.Color.Static900 else LoomarrTokens.Color.Static950)
                .border(
                    if (focused) 3.dp else 1.dp,
                    if (focused) LoomarrTokens.Color.Signal else LoomarrTokens.Color.Static950,
                    RoundedCornerShape(LoomarrTokens.Radius.Lg),
                ).padding(horizontal = LoomarrTokens.Space.S4, vertical = LoomarrTokens.Space.S3),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            MonoData(
                channel.number.toString().padStart(2, '0'),
                color = if (focused) LoomarrTokens.Color.Signal else LoomarrTokens.Color.Static400,
            )
            Body(
                channel.name,
                color = if (focused) LoomarrTokens.Color.Static0 else LoomarrTokens.Color.Static100,
                fontSize = LoomarrTokens.Type.Sm,
                maxLines = 1,
                modifier = Modifier.padding(start = LoomarrTokens.Space.S3).weight(1f),
            )
            if (watching) {
                Box(
                    modifier =
                        Modifier
                            .padding(start = LoomarrTokens.Space.S3)
                            .width(8.dp)
                            .height(8.dp)
                            .clip(RoundedCornerShape(LoomarrTokens.Radius.Lg))
                            .background(LoomarrTokens.Color.Lock),
                )
            }
        }
        if (focused) {
            val current = info?.current
            Row(
                modifier = Modifier.padding(start = LoomarrTokens.Space.S8, top = LoomarrTokens.Space.S1),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Body(
                    current?.episodeTitle ?: current?.heading ?: "Nothing scheduled",
                    fontSize = LoomarrTokens.Type.Xs,
                    maxLines = 1,
                    modifier = Modifier.weight(1f),
                )
                if (current != null) {
                    MonoData(
                        "${info.minutesRemaining}m left",
                        color = LoomarrTokens.Color.Static500,
                        fontSize = LoomarrTokens.Type.Xs2,
                        maxLines = 1,
                        modifier = Modifier.padding(start = LoomarrTokens.Space.S2),
                    )
                }
            }
            Box(
                modifier =
                    Modifier
                        .padding(start = LoomarrTokens.Space.S8, top = LoomarrTokens.Space.S2)
                        .fillMaxWidth()
                        .height(4.dp)
                        .clip(RoundedCornerShape(LoomarrTokens.Radius.Sm))
                        .background(LoomarrTokens.Color.Static950),
            ) {
                Box(
                    modifier =
                        Modifier
                            .fillMaxWidth(info?.progress ?: 0f)
                            .fillMaxHeight()
                            .background(LoomarrTokens.Color.Signal),
                )
            }
        }
    }
}

private data class ChannelGuideInfo(
    val channel: ChannelTimeline,
    val current: Airing?,
    val next: Airing?,
    val progress: Float,
) {
    val minutesRemaining: Int
        get() =
            current
                ?.let { (((1f - progress) * it.durationMs) / 60_000f).toInt().coerceAtLeast(1) }
                ?: 0
}

private fun GuideUiState.infoFor(channelId: String): ChannelGuideInfo? {
    val ready = this as? GuideUiState.Ready ?: return null
    val channel = ready.window.channels.firstOrNull { it.channelId == channelId } ?: return null
    val current = channel.airingAt(ready.nowMs)
    val next = channel.airings.firstOrNull { it.startMs >= (current?.stopMs ?: ready.nowMs) }
    val progress =
        current?.let {
            ((ready.nowMs - it.startMs).toFloat() / it.durationMs.toFloat()).coerceIn(0f, 1f)
        } ?: 0f
    return ChannelGuideInfo(channel, current, next, progress)
}

internal fun remoteDigit(key: Key): Char? =
    when (key) {
        Key.Zero -> '0'
        Key.One -> '1'
        Key.Two -> '2'
        Key.Three -> '3'
        Key.Four -> '4'
        Key.Five -> '5'
        Key.Six -> '6'
        Key.Seven -> '7'
        Key.Eight -> '8'
        Key.Nine -> '9'
        else -> null
    }

private sealed interface SurfRow {
    data class Heading(
        val title: String,
        val count: Int,
    ) : SurfRow

    data class Empty(
        val section: String,
    ) : SurfRow

    data class ChannelRow(
        val channel: Channel,
    ) : SurfRow
}

private val SURF_RAIL_WIDTH = 420.dp
private const val MAX_CHANNEL_DIGITS = 3
private const val NUMBER_ENTRY_MS = 1_200L
private const val BANNER_VISIBLE_MS = 5_000L
