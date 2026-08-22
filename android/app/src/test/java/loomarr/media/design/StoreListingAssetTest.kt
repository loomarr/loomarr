package loomarr.media.design

import android.graphics.BitmapFactory
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import java.io.File

@RunWith(RobolectricTestRunner::class)
class StoreListingAssetTest {
    private val storeListingRoot = File("../store-listing")

    @Test
    fun `Play listing artwork has exact dimensions and no alpha channel`() {
        val expectedDimensions =
            mapOf(
                "play-icon-512x512.png" to (512 to 512),
                "feature-graphic-1024x500.png" to (1024 to 500),
                "tv-banner-1280x720.png" to (1280 to 720),
            )

        expectedDimensions.forEach { (name, dimensions) ->
            val file = storeListingRoot.resolve(name)
            val image = BitmapFactory.decodeFile(file.path)

            assertNotNull("$file must be a readable image", image)
            assertEquals("$name width", dimensions.first, image.width)
            assertEquals("$name height", dimensions.second, image.height)
            assertFalse("$name must be opaque", image.hasAlpha())
        }
    }
}
