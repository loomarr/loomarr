package tv.loomarr.tv

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Text
import kotlinx.coroutines.runBlocking
import tv.loomarr.tv.pairing.DeviceStore
import tv.loomarr.tv.pairing.PairingUiState
import tv.loomarr.tv.pairing.PairingViewModel
import tv.loomarr.tv.pairing.PairingViewModelFactory

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val store = DeviceStore(applicationContext)

        // Development affordance: `adb shell am start -n tv.loomarr.tv/.MainActivity -e server
        // http://10.0.2.2:18305` sets the address without a keyboard. A TV cannot practically type
        // a URL, so the shipped path will be discovery or an on-screen entry step — this exists so
        // the app is testable before that lands, and it is inert when the extra is absent.
        intent?.getStringExtra("server")?.takeIf { it.isNotBlank() }?.let { url ->
            runBlocking { store.setServerUrl(url) }
        }

        setContent {
            MaterialTheme {
                PairingScreen(
                    model = viewModel(factory = PairingViewModelFactory(store)),
                )
            }
        }
    }
}

/**
 * The first screen a TV shows: where to go, and what to type.
 *
 * Designed for a 10-foot read. The code is the largest thing on screen because it is the one piece
 * of information someone has to carry across the room to a phone, and the address is second because
 * it is useless without knowing where to type the code.
 *
 * ⚠ Overscan: older TVs crop the edges of the picture, so nothing sits within 48dp of the border.
 * That margin is convention on Android TV, not caution.
 */
@Composable
private fun PairingScreen(model: PairingViewModel = viewModel()) {
    val state by model.state.collectAsStateWithLifecycle()

    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(Color(0xFF0B0F19))
            .padding(48.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        when (val current = state) {
            is PairingUiState.Loading -> Text(
                text = "Starting…",
                fontSize = 28.sp,
                color = Color.White,
            )

            is PairingUiState.NeedsServer -> {
                Text(text = "Loomarr", fontSize = 40.sp, color = Color.White)
                Text(
                    text = "No server address is set for this device yet.",
                    fontSize = 24.sp,
                    color = Color(0xFF9CA3AF),
                    modifier = Modifier.padding(top = 24.dp),
                    textAlign = TextAlign.Center,
                )
            }

            is PairingUiState.AwaitingApproval -> {
                Text(
                    text = "Set up Loomarr",
                    fontSize = 40.sp,
                    color = Color.White,
                )
                Text(
                    text = "On your phone or computer, go to",
                    fontSize = 24.sp,
                    color = Color(0xFF9CA3AF),
                    modifier = Modifier.padding(top = 32.dp),
                    textAlign = TextAlign.Center,
                )
                Text(
                    text = current.verificationUri,
                    fontSize = 34.sp,
                    color = Color.White,
                )
                Text(
                    text = "and enter this code",
                    fontSize = 24.sp,
                    color = Color(0xFF9CA3AF),
                    modifier = Modifier.padding(top = 32.dp),
                )
                // The code is the payload of this entire screen.
                Text(
                    text = current.userCode,
                    fontSize = 88.sp,
                    color = Color(0xFFF59E0B),
                    modifier = Modifier.padding(top = 8.dp),
                )
            }

            is PairingUiState.Paired -> Text(
                text = "${current.deviceName} is ready",
                fontSize = 40.sp,
                color = Color.White,
            )

            is PairingUiState.Failed -> Text(
                text = current.message,
                fontSize = 28.sp,
                color = Color(0xFFF87171),
                textAlign = TextAlign.Center,
            )
        }
    }
}
