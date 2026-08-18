package tv.loomarr.tv.design

import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Guards the design system by reading the source, the way the web module's coverage test guards
 * colocated tests.
 *
 * ⚠ This exists because the failure it catches is invisible at runtime. A screen that writes
 * `fontSize = 32.sp` renders perfectly — it just quietly stops tracking the design system, and the
 * next person copies it. That is exactly how `#F59E0B` (a Tailwind amber) shipped in place of
 * Loomarr's `#FFB020`, and nothing caught it until the tokens were generated.
 *
 * Only `design/` may name raw values; every other package composes what it defines.
 */
class DesignSystemUsageTest {
    private val sourceRoot = File("src/main/java/tv/loomarr/tv")

    private fun screenSources(): List<File> =
        sourceRoot
            .walkTopDown()
            .filter { it.isFile && it.extension == "kt" }
            // The design package IS where raw values are declared, so it is the one exemption.
            .filter { !it.path.contains("/design/") }
            .toList()

    @Test
    fun `no screen hardcodes a colour`() {
        val offenders =
            screenSources().filter { file ->
                file.readLines().any { line ->
                    val code = line.substringBefore("//")
                    Regex("""Color\(0x[0-9A-Fa-f]{8}\)""").containsMatchIn(code)
                }
            }

        assertTrue(
            "these files name a raw colour instead of a LoomarrTokens.Color value: " +
                offenders.joinToString { it.name },
            offenders.isEmpty(),
        )
    }

    @Test
    fun `no screen hardcodes a font size`() {
        val offenders =
            screenSources().filter { file ->
                file.readLines().any { line ->
                    val code = line.substringBefore("//")
                    Regex("""fontSize\s*=\s*\d+\.sp""").containsMatchIn(code)
                }
            }

        assertTrue(
            "these files set a raw fontSize instead of using a text component: " +
                offenders.joinToString { it.name },
            offenders.isEmpty(),
        )
    }

    /**
     * Overscan is the one measurement a television imposes and the web has no equivalent for, so it
     * lives in `design/` as a single constant. A screen padding itself by a literal 48dp has
     * re-derived it, and will not follow if the value ever changes.
     */
    @Test
    fun `no screen re-derives the overscan margin`() {
        val offenders =
            screenSources().filter { file ->
                file.readLines().any { line ->
                    val code = line.substringBefore("//")
                    Regex("""padding\(\s*48\.dp""").containsMatchIn(code)
                }
            }

        assertTrue(
            "these files hardcode the overscan margin instead of composing Screen(): " +
                offenders.joinToString { it.name },
            offenders.isEmpty(),
        )
    }
}
