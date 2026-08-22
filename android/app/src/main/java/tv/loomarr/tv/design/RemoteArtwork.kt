package tv.loomarr.tv.design

import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.IOException
import java.util.LinkedHashMap

/** A bounded, process-local cache for public image-service renditions used by TV chrome. */
internal class ArtworkLoader(
    private val http: OkHttpClient = OkHttpClient(),
) {
    private val maxEntries = 24
    private val images =
        object : LinkedHashMap<String, ImageBitmap>(maxEntries, 0.75f, true) {
            override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, ImageBitmap>?): Boolean =
                size > maxEntries
        }

    suspend fun load(
        url: String,
        authorization: String?,
    ): ImageBitmap? =
        withContext(Dispatchers.IO) {
            synchronized(images) { images[url] }?.let { return@withContext it }
            val request =
                Request
                    .Builder()
                    .url(url)
                    .apply {
                        authorization?.takeIf { it.isNotBlank() }?.let {
                            header("Authorization", it)
                        }
                    }.build()
            val loaded =
                runCatching {
                    http.newCall(request).execute().use { response ->
                        if (!response.isSuccessful) throw IOException("artwork failed: ${response.code}")
                        val bytes = response.body?.bytes() ?: return@use null
                        BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
                    }
                }.getOrNull()
            if (loaded != null) synchronized(images) { images[url] = loaded }
            loaded
        }
}

private val ArtworkImages = ArtworkLoader()

/** Same-origin programme artwork with a stable frame and an intentional no-art state. */
@Composable
fun RemoteArtwork(
    url: String?,
    title: String,
    modifier: Modifier = Modifier,
    authorization: String? = null,
) {
    val artwork = produceState<ImageBitmap?>(initialValue = null, key1 = url, key2 = authorization) {
        value = url?.let { ArtworkImages.load(it, authorization) }
    }.value
    val shape = RoundedCornerShape(LoomarrTokens.Radius.Md)

    Box(
        modifier =
            modifier
                .clip(shape)
                .background(LoomarrTokens.Color.Static800)
                .border(1.dp, LoomarrTokens.Color.Static700, shape),
        contentAlignment = Alignment.Center,
    ) {
        if (artwork != null) {
            Image(
                bitmap = artwork,
                contentDescription = title,
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxSize(),
            )
        } else {
            MonoData(
                "NO ART",
                color = LoomarrTokens.Color.Static500,
                fontSize = LoomarrTokens.Type.Xs2,
                maxLines = 1,
            )
        }
    }
}
