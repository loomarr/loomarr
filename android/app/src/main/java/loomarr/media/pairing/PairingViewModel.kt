package loomarr.media.pairing

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** What the television is showing right now. */
sealed interface PairingUiState {
    data object Loading : PairingUiState

    /**
     * No server address is stored yet, so there is nothing to pair with.
     *
     * A TV cannot practically type a URL, so this state exists to SHOW the problem rather than
     * fail silently — a first-run appliance with no address configured is otherwise a black screen
     * with no explanation. `adb shell am start -e server <url>` fills it in during development.
     */
    data object NeedsServer : PairingUiState

    /**
     * A code is on screen and the device is polling.
     *
     * Two URIs because RFC 8628 defines two and they do different jobs: [verificationUri] is the
     * short address a human types, [verificationUriComplete] carries the code for a QR to encode.
     * Showing the complete one as text would be worse than useless — longer to read, and the code
     * is already displayed on its own.
     */
    data class AwaitingApproval(
        val userCode: String,
        val verificationUri: String,
        val verificationUriComplete: String,
        /** Seconds left before this code stops working; drives the countdown. */
        val secondsRemaining: Long,
    ) : PairingUiState

    data class Paired(
        val deviceName: String,
    ) : PairingUiState

    data class Failed(
        val message: String,
    ) : PairingUiState
}

/**
 * Drives the pairing handshake and nothing else.
 *
 * The loop restarts itself when a code expires rather than stranding the viewer on a dead code —
 * an abandoned TV showing an unusable code with no way forward is the failure mode this whole
 * screen exists to avoid.
 */
class PairingViewModel(
    private val store: DeviceStore,
    private val clientFor: (baseUrl: String) -> PairingClient = { PairingClient(it) },
) : ViewModel() {
    private val _state = MutableStateFlow<PairingUiState>(PairingUiState.Loading)
    val state: StateFlow<PairingUiState> = _state.asStateFlow()

    init {
        beginPairing()
    }

    /**
     * Ticks [PairingUiState.AwaitingApproval.secondsRemaining] down once a second.
     *
     * Held so a new pairing can cancel the previous ticker. Without that, asking for a fresh code
     * leaves two coroutines decrementing the same field and the countdown runs at double speed —
     * the classic symptom of a restarted timer that was never stopped.
     */
    private var countdown: Job? = null

    private fun startCountdown() {
        countdown?.cancel()
        countdown =
            viewModelScope.launch {
                while (true) {
                    delay(1_000)
                    val current = _state.value as? PairingUiState.AwaitingApproval ?: return@launch
                    if (current.secondsRemaining <= 0) return@launch
                    _state.value = current.copy(secondsRemaining = current.secondsRemaining - 1)
                }
            }
    }

    /**
     * Throw the current code away and ask for another.
     *
     * The screen offers this because a code CAN be missed — someone walks away, the ten minutes
     * lapse, or the digits were mistyped once too often. Without it the only recovery is restarting
     * the app, which is not something a viewer should have to work out from a remote.
     */
    fun refreshCode() {
        countdown?.cancel()
        pairingJob?.cancel()
        _state.value = PairingUiState.Loading
        beginPairing()
    }

    private var pairingJob: Job? = null

    private fun beginPairing() {
        pairingJob?.cancel()
        pairingJob =
            viewModelScope.launch {
                val baseUrl = store.serverUrl()
                if (baseUrl.isNullOrBlank()) {
                    _state.value = PairingUiState.NeedsServer
                    return@launch
                }
                // Already paired from a previous launch — nothing to do on this screen.
                store.token()?.let {
                    _state.value = PairingUiState.Paired(android.os.Build.MODEL ?: "This device")
                    return@launch
                }
                val client = clientFor(baseUrl)
                while (true) {
                    val pairing =
                        try {
                            client.start(deviceName = android.os.Build.MODEL ?: "Android TV")
                        } catch (error: Exception) {
                            // Include the underlying reason. A bare "can't reach it" sent the first
                            // emulator run chasing the network when the real cause was Android's cleartext
                            // block — the exception said so, and the UI threw that away.
                            _state.value =
                                PairingUiState.Failed(
                                    "Can't reach Loomarr at $baseUrl\n${error.message ?: error::class.simpleName}",
                                )
                            return@launch
                        }

                    _state.value =
                        PairingUiState.AwaitingApproval(
                            userCode = pairing.userCode,
                            // The human-facing half of RFC 8628: the address to type, matching the web
                            // app's /pair route. Scheme stripped because nobody types "http://".
                            verificationUri = "${baseUrl.removePrefix("http://").removePrefix("https://")}/pair",
                            // ⚠ The QR needs the FULL url, scheme included — a scanner handed
                            // "host/pair?code=…" has no protocol to open. This is the one place the
                            // scheme must survive.
                            verificationUriComplete = "$baseUrl/pair?code=${pairing.userCode}",
                            secondsRemaining = pairing.expiresInSeconds,
                        )
                    startCountdown()

                    when (val outcome = pollUntilSettled(client, pairing)) {
                        is PollResult.Paired -> {
                            // Persist BEFORE reporting success: a token shown as paired but not stored
                            // would be lost on the next launch, and the pairing that produced it is
                            // already consumed server-side, so there would be no way to recover it.
                            store.setToken(outcome.token)
                            _state.value = PairingUiState.Paired(outcome.deviceName)
                            return@launch
                        }
                        // Expired: loop around and mint a fresh code rather than leaving a dead one on
                        // screen. Nobody watching can tell an expired code from a working one.
                        PollResult.Expired -> continue
                        PollResult.Pending -> return@launch // unreachable; pollUntilSettled never returns it
                    }
                }
            }
    }

    private suspend fun pollUntilSettled(
        client: PairingClient,
        pairing: Pairing,
    ): PollResult {
        while (true) {
            delay(pairing.intervalSeconds.coerceAtLeast(1) * 1000L)
            val result =
                try {
                    client.poll(pairing.deviceCode)
                } catch (error: Exception) {
                    // A transient network failure is not a dead code. Keep waiting — the alternative
                    // discards a code the viewer may already be typing.
                    continue
                }
            if (result != PollResult.Pending) return result
        }
    }
}

/**
 * Builds [PairingViewModel] with its dependencies.
 *
 * ⚠ Required, not ceremony. `viewModel()` with no factory reflects on a NO-ARG constructor, so a
 * ViewModel that takes arguments compiles fine and then throws
 * `Cannot create an instance of class …` the moment the screen renders. The type checker cannot
 * see it because `viewModel()` is generic — only running the app does.
 */
class PairingViewModelFactory(
    private val store: DeviceStore,
) : ViewModelProvider.Factory {
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        require(modelClass.isAssignableFrom(PairingViewModel::class.java)) {
            "unexpected ViewModel: ${modelClass.name}"
        }
        @Suppress("UNCHECKED_CAST")
        return PairingViewModel(store) as T
    }
}
