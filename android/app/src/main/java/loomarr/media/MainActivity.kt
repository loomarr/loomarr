package loomarr.media

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.runBlocking
import loomarr.media.design.Body
import loomarr.media.design.BrandLockup
import loomarr.media.design.CenteredScreen
import loomarr.media.design.CodeDisplay
import loomarr.media.design.ColorBars
import loomarr.media.design.Countdown
import loomarr.media.design.ErrorText
import loomarr.media.design.Heading
import loomarr.media.design.HorizontalDivider
import loomarr.media.design.LoomarrTokens
import loomarr.media.design.MonoData
import loomarr.media.design.Panel
import loomarr.media.design.QrCode
import loomarr.media.design.SectionHeading
import loomarr.media.design.TuningText
import loomarr.media.design.TvButton
import loomarr.media.design.VerticalDivider
import loomarr.media.guide.GuideScreen
import loomarr.media.guide.GuideViewModel
import loomarr.media.guide.GuideViewModelFactory
import loomarr.media.navigation.TvHomeState
import loomarr.media.navigation.TvSurface
import loomarr.media.pairing.DeviceStore
import loomarr.media.pairing.PairingUiState
import loomarr.media.pairing.PairingViewModel
import loomarr.media.pairing.PairingViewModelFactory
import loomarr.media.playback.ChannelCatalogState
import loomarr.media.playback.ChannelCatalogViewModel
import loomarr.media.playback.ChannelCatalogViewModelFactory
import loomarr.media.playback.WatchScreen
import loomarr.media.playback.WatchUiState
import loomarr.media.playback.WatchViewModel
import loomarr.media.playback.WatchViewModelFactory

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val store = DeviceStore(applicationContext)

        // Development affordance: `adb shell am start -n
        // loomarr.media.debug/loomarr.media.MainActivity -e server
        // http://10.0.2.2:18305` sets the address without a keyboard. A TV cannot practically type a
        // URL, so the shipped path will be discovery or an on-screen entry step — this exists so the
        // app is testable before that lands, and it is inert when the extra is absent.
        intent?.getStringExtra("server")?.takeIf { it.isNotBlank() }?.let { url ->
            runBlocking { store.setServerUrl(url) }
        }

        setContent {
            MaterialTheme {
                LoomarrApp(store)
            }
        }
    }
}

/**
 * The whole app, which is two screens: pair, then watch.
 *
 * Routing is derived from pairing state rather than a navigation graph. A television has exactly one
 * decision to make at launch — am I paired? — and the answer already lives in [PairingUiState], so a
 * router here would be a second copy of it that could disagree.
 */
@Composable
private fun LoomarrApp(store: DeviceStore) {
    val pairing: PairingViewModel = viewModel(factory = PairingViewModelFactory(store))
    val state by pairing.state.collectAsStateWithLifecycle()

    if (state is PairingUiState.Paired) {
        PairedApp(store)
    } else {
        PairingScreen(model = pairing)
    }
}

/**
 * The first screen a TV shows: where to go, and what to type.
 *
 * Every size, colour and margin comes from the design system — [CenteredScreen] owns the overscan
 * margin and the background, and the text components own the scale. This screen states WHAT it is
 * showing; it no longer decides how big anything is.
 */
@Composable
private fun PairingScreen(model: PairingViewModel) {
    val state by model.state.collectAsStateWithLifecycle()

    CenteredScreen {
        // ⚠ While a code is on screen the mark shrinks to the bare strip: no wordmark, no tagline.
        //
        // The pairing panel now carries a QR, an address, the code, a countdown and a control, and
        // the full lockup on top of that left ten pixels of headroom — which is how the button and
        // countdown ended up clipped. The brand does not need to introduce itself on the screen a
        // viewer is already staring at; the strip alone is recognisably Loomarr, and the payload
        // gets the room.
        if (state is PairingUiState.AwaitingApproval) {
            ColorBars(modifier = Modifier.padding(bottom = LoomarrTokens.Space.S6))
        } else {
            BrandLockup(modifier = Modifier.padding(bottom = LoomarrTokens.Space.S6))
        }

        when (val current = state) {
            // "Tuning in…" rather than "Starting…" — nostalgia lives in the microcopy
            // (frontend-design §1), and this is a television.
            is PairingUiState.Loading -> TuningText("Tuning in…")

            is PairingUiState.NeedsServer ->
                Body(
                    "No server address is set for this device yet.",
                    align = TextAlign.Center,
                )

            is PairingUiState.AwaitingApproval ->
                PairingOffer(state = current, onRefresh = model::refreshCode)

            // "You're on the air" is the product's own phrase for this moment (frontend-design §1).
            is PairingUiState.Paired -> Heading("${current.deviceName} is on the air")

            is PairingUiState.Failed -> ErrorText(current.message)
        }
    }
}

/**
 * The pairing offer: scan on the left, type on the right, divided down the middle.
 *
 * Two equal halves rather than a primary and a fallback, because they genuinely suit different
 * people — someone with their phone in hand scans, someone at a laptop types. The rule between them
 * says "either of these", where a stacked layout would have implied "this, then that".
 *
 * Below the rule sits the countdown and the way out. A code expires after ten minutes, and until
 * now the only recovery from a missed one was restarting the app — which is not something a viewer
 * can be expected to work out from a remote.
 */
@Composable
internal fun PairingOffer(
    state: PairingUiState.AwaitingApproval,
    onRefresh: () -> Unit,
) {
    // The button is the screen's only control, so it takes focus on arrival. Without this the first
    // D-pad press goes nowhere and the remote feels broken.
    val focus = remember { FocusRequester() }
    LaunchedEffect(Unit) { focus.requestFocus() }

    // ⚠ An explicit width rather than the inherited full width. CenteredScreen's column is
    // fillMaxSize, so without this the panel spans the screen edge to edge — the QR pinned far left,
    // the code far right, and the divider nowhere near the middle.
    //
    // Stated rather than intrinsic: the two halves must be EQUAL for the divider to land on the
    // true centre, and an intrinsic width would size them to their content instead, putting the
    // rule wherever the longer side happened to end.
    Panel(
        modifier = Modifier.width(760.dp),
        padding = LoomarrTokens.Space.S6,
    ) {
        // ⚠ Top-aligned, not centre-aligned. Each half now leads with a heading, and those headings
        // must sit on the same line for the two options to read as parallel choices — centring the
        // halves would float them against each other's differing content heights.
        Row(verticalAlignment = Alignment.Top) {
            // Equal weights so each half owns exactly half the panel, which puts the divider on the
            // true centre line regardless of how long the address or code happen to be.
            Column(
                modifier = Modifier.weight(1f),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(LoomarrTokens.Space.S4),
            ) {
                SectionHeading("Scan QR Code")
                QrCode(content = state.verificationUriComplete, size = 150.dp)
            }

            VerticalDivider(height = 210.dp)

            Column(
                modifier = Modifier.weight(1f),
                horizontalAlignment = Alignment.CenterHorizontally,
                // ⚠ Spaced rather than stacked. The address and code were butting against each
                // other with only 4dp between them, which is what made this side feel tight — the
                // left half is a single object with natural whitespace, so the right needed its own.
                verticalArrangement = Arrangement.spacedBy(LoomarrTokens.Space.S4),
            ) {
                SectionHeading("Visit Website")
                MonoData(
                    state.verificationUri,
                    fontSize = LoomarrTokens.Type.Xs2,
                    maxLines = 1,
                )
                CodeDisplay(state.userCode)
            }
        }

        HorizontalDivider(modifier = Modifier.padding(vertical = LoomarrTokens.Space.S5))

        Column(
            modifier = Modifier.fillMaxWidth(),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Countdown(secondsRemaining = state.secondsRemaining)
            TvButton(
                text = "Get a new code",
                onClick = onRefresh,
                modifier = Modifier.padding(top = LoomarrTokens.Space.S3),
                focusRequester = focus,
            )
        }
    }
}

/**
 * The watching-first paired shell: Watching, its Surf overlay, and Guide.
 *
 * [TvHomeState] keeps those three remote states explicit without adding a navigation dependency or
 * duplicating playback state. Watching is the entry point; Surf leaves its player mounted, while
 * Guide is the full-screen browse surface and always returns to Watching after Back or a tune.
 */
@Composable
private fun PairedApp(store: DeviceStore) {
    var home by remember { mutableStateOf(TvHomeState()) }
    val catalogModel: ChannelCatalogViewModel =
        viewModel(factory = ChannelCatalogViewModelFactory(store))
    val catalog = catalogModel.catalog
    val catalogState by catalog.state.collectAsStateWithLifecycle()
    val lifecycleOwner = LocalLifecycleOwner.current

    // The stream exists only while the TV is foregrounded. Every start also performs a complete
    // reconciliation, so an event missed while asleep cannot leave the lineup stale.
    DisposableEffect(lifecycleOwner, catalog) {
        val observer =
            LifecycleEventObserver { _, event ->
                when (event) {
                    Lifecycle.Event.ON_START -> catalog.start()
                    Lifecycle.Event.ON_STOP -> catalog.stop()
                    else -> Unit
                }
            }
        lifecycleOwner.lifecycle.addObserver(observer)
        if (lifecycleOwner.lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) catalog.start()
        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
            catalog.stop()
        }
    }

    val watch: WatchViewModel = viewModel(factory = WatchViewModelFactory(store, catalog))
    val watchState by watch.state.collectAsStateWithLifecycle()
    val guide: GuideViewModel = viewModel(factory = GuideViewModelFactory(store, catalog))

    if (home.surface == TvSurface.Guide) {
        GuideScreen(
            model = guide,
            onTune = { channel ->
                watch.tuneChannelId(channel.channelId)
                home = home.watch()
            },
            onBack = { home = home.watch() },
            recentChannelIds =
                (watchState as? WatchUiState.Ready)?.recentChannelIds
                    ?: emptyList(),
            playableChannelIds =
                (catalogState as? ChannelCatalogState.Ready)
                    ?.channels
                    ?.mapTo(mutableSetOf()) { it.id },
        )
    } else {
        WatchScreen(
            model = watch,
            guideModel = guide,
            showingSurf = home.surfVisible,
            onOpenGuide = {
                guide.load()
                home = home.openGuide()
            },
            onOpenSurf = { home = home.openSurf() },
            onCloseSurf = { home = home.closeOverlay() },
        )
    }
}
