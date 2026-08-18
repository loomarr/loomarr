package tv.loomarr.tv

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.runBlocking
import tv.loomarr.tv.design.Body
import tv.loomarr.tv.design.BrandLockup
import tv.loomarr.tv.design.CenteredScreen
import tv.loomarr.tv.design.CodeDisplay
import tv.loomarr.tv.design.ErrorText
import tv.loomarr.tv.design.Heading
import tv.loomarr.tv.design.LoomarrTokens
import tv.loomarr.tv.design.MonoData
import tv.loomarr.tv.design.Panel
import tv.loomarr.tv.design.QrCode
import tv.loomarr.tv.design.TuningText
import tv.loomarr.tv.pairing.DeviceStore
import tv.loomarr.tv.pairing.PairingUiState
import tv.loomarr.tv.pairing.PairingViewModel
import tv.loomarr.tv.pairing.PairingViewModelFactory
import tv.loomarr.tv.playback.WatchScreen
import tv.loomarr.tv.playback.WatchViewModelFactory

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val store = DeviceStore(applicationContext)

        // Development affordance: `adb shell am start -n tv.loomarr.tv/.MainActivity -e server
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
        WatchScreen(model = viewModel(factory = WatchViewModelFactory(store)))
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
        // ⚠ The mark drops its tagline while a code is on screen. The lockup plus the instructions
        // plus an 88sp code is taller than 1080p minus the overscan margin, and the first build
        // clipped the code mid-glyph — the one element a viewer must read exactly. The brand gives
        // up a line before the payload does.
        BrandLockup(
            modifier = Modifier.padding(bottom = LoomarrTokens.Space.S6),
            tagline = state !is PairingUiState.AwaitingApproval,
        )

        when (val current = state) {
            // "Tuning in…" rather than "Starting…" — nostalgia lives in the microcopy
            // (frontend-design §1), and this is a television.
            is PairingUiState.Loading -> TuningText("Tuning in…")

            is PairingUiState.NeedsServer ->
                Body(
                    "No server address is set for this device yet.",
                    align = TextAlign.Center,
                )

            is PairingUiState.AwaitingApproval -> {
                // ONE instruction, naming the address inline. The earlier layout had three separate
                // lines of guidance around two competing columns, and read as a wall rather than a
                // single next step.
                Body("On your phone, go to", align = TextAlign.Center)
                MonoData(
                    current.verificationUri,
                    modifier = Modifier.padding(top = LoomarrTokens.Space.S1),
                )

                // ⚠ The code and the QR are ONE object, not two columns — a bordered panel, both
                // halves on a shared centre line, with the code first because it always works.
                // Scanning depends on the phone being able to reach the server: on a Tailscale or
                // VPN address a phone off the network gets a dead link, which is worse than typing
                // because it fails silently.
                // ⚠ Sized against the height budget, not by eye. At 1080p with a 48dp overscan
                // margin the usable height is 888px, and the first version totalled 896 — eight
                // pixels over, so the QR's bottom edge clipped. A 140dp code and the tighter panel
                // padding bring the stack back inside it with room to spare.
                Panel(
                    modifier = Modifier.padding(top = LoomarrTokens.Space.S4),
                    padding = LoomarrTokens.Space.S4,
                ) {
                    Row(
                        horizontalArrangement = Arrangement.spacedBy(LoomarrTokens.Space.S6),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        CodeDisplay(current.userCode)
                        QrCode(content = current.verificationUriComplete, size = 140.dp)
                    }
                }
            }

            // "You're on the air" is the product's own phrase for this moment (frontend-design §1).
            is PairingUiState.Paired -> Heading("${current.deviceName} is on the air")

            is PairingUiState.Failed -> ErrorText(current.message)
        }
    }
}
