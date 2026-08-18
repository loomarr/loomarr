package tv.loomarr.tv.pairing

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** What the television is showing right now. */
sealed interface PairingUiState {
    data object Loading : PairingUiState

    /** A code is on screen and the device is polling. */
    data class AwaitingApproval(val userCode: String, val verificationUri: String) : PairingUiState

    data class Paired(val deviceName: String) : PairingUiState

    data class Failed(val message: String) : PairingUiState
}

/**
 * Drives the pairing handshake and nothing else.
 *
 * The loop restarts itself when a code expires rather than stranding the viewer on a dead code —
 * an abandoned TV showing an unusable code with no way forward is the failure mode this whole
 * screen exists to avoid.
 */
class PairingViewModel(
    private val client: PairingClient,
    private val baseUrl: String,
    private val onPaired: suspend (token: String) -> Unit = {},
) : ViewModel() {

    private val _state = MutableStateFlow<PairingUiState>(PairingUiState.Loading)
    val state: StateFlow<PairingUiState> = _state.asStateFlow()

    init {
        beginPairing()
    }

    private fun beginPairing() {
        viewModelScope.launch {
            while (true) {
                val pairing = try {
                    client.start(deviceName = android.os.Build.MODEL ?: "Android TV")
                } catch (error: Exception) {
                    _state.value = PairingUiState.Failed(
                        "Can't reach Loomarr at $baseUrl. Check the address and your network.",
                    )
                    return@launch
                }

                _state.value = PairingUiState.AwaitingApproval(
                    userCode = pairing.userCode,
                    // The human-facing half of RFC 8628: the address to type, matching the web
                    // app's /pair route.
                    verificationUri = "${baseUrl.removePrefix("http://").removePrefix("https://")}/pair",
                )

                when (val outcome = pollUntilSettled(pairing)) {
                    is PollResult.Paired -> {
                        onPaired(outcome.token)
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

    private suspend fun pollUntilSettled(pairing: Pairing): PollResult {
        while (true) {
            delay(pairing.intervalSeconds.coerceAtLeast(1) * 1000L)
            val result = try {
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
