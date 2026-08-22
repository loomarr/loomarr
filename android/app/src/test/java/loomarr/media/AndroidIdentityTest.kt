package loomarr.media

import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class AndroidIdentityTest {
    private val expectedNamespace = "loomarr.media"

    @Test
    fun `Gradle and every Kotlin source use the permanent identity`() {
        val build = File("build.gradle.kts").readText()
        assertTrue(build.contains("namespace = \"$expectedNamespace\""))
        assertTrue(build.contains("applicationId = \"$expectedNamespace\""))

        val offenders =
            listOf(File("src/main/java"), File("src/test/java"))
                .flatMap { root -> root.walkTopDown().filter { it.isFile && it.extension == "kt" }.toList() }
                .filter { source ->
                    val declaration =
                        source.useLines { lines ->
                            lines.firstOrNull { it.startsWith("package ") }
                        }
                    declaration == null ||
                        (
                            declaration != "package $expectedNamespace" &&
                                !declaration.startsWith("package $expectedNamespace.")
                        )
                }

        assertTrue(
            "Kotlin sources outside $expectedNamespace: ${offenders.joinToString { it.path }}",
            offenders.isEmpty(),
        )
    }
}
