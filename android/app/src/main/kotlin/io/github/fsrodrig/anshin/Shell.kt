package io.github.fsrodrig.anshin

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.semantics.text
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.compose.ui.semantics.isTraversalGroup
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.traversalIndex
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** The scrolling region of every screen, so a test can scroll any screen the same way. */
internal const val SCREEN_CONTENT_TAG = "screen-content"

/** Where a screen's back arrow goes. TalkBack reads it as "Back to [to]", so the Owner hears where Back lands. */
@Immutable
internal class Back(val to: String, val go: () -> Unit)

/**
 * Every screen: a bar, an optional [pinned] line under it, then [content], the only part that scrolls. With no [back]
 * this is the root, the Dashboard, whose bar carries the lockup instead of a back arrow and a [title].
 */
@Composable
internal fun AnshinScreen(
    title: String?,
    back: Back?,
    modifier: Modifier = Modifier,
    actions: @Composable RowScope.() -> Unit = {},
    pinned: (@Composable () -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    val scroll = rememberScrollState()
    Frame(title, back, modifier, actions, pinned, scrolled = { scroll.value > 0 }) { body ->
        Column(
            body.verticalScroll(scroll).padding(ContentPadding),
            verticalArrangement = Arrangement.spacedBy(16.dp),
            content = content,
        )
    }
}

/** As [AnshinScreen], for content long enough to be a lazy list held in [list]. */
@Composable
internal fun AnshinListScreen(
    title: String?,
    back: Back?,
    modifier: Modifier = Modifier,
    list: LazyListState = rememberLazyListState(),
    actions: @Composable RowScope.() -> Unit = {},
    pinned: (@Composable () -> Unit)? = null,
    itemSpacing: Dp = 12.dp,
    content: LazyListScope.() -> Unit,
) {
    val scrolled = { list.firstVisibleItemIndex > 0 || list.firstVisibleItemScrollOffset > 0 }
    Frame(title, back, modifier, actions, pinned, scrolled) { body ->
        LazyColumn(body, state = list, contentPadding = ContentPadding, verticalArrangement = Arrangement.spacedBy(itemSpacing), content = content)
    }
}

private val ContentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 24.dp)

@Composable
private fun Frame(
    title: String?,
    back: Back?,
    modifier: Modifier,
    actions: @Composable RowScope.() -> Unit,
    pinned: (@Composable () -> Unit)?,
    scrolled: () -> Boolean,
    body: @Composable (Modifier) -> Unit,
) {
    val colors = AnshinTheme.colors
    val root = back == null
    Column(modifier.fillMaxSize().background(colors.background).safeDrawingPadding()) {
        Column(
            Modifier
                .fillMaxWidth()
                // Read while drawing, so scrolling redraws the rule without recomposing or relaying out the bar.
                .drawWithContent {
                    drawContent()
                    if (scrolled()) {
                        val y = size.height - 0.5.dp.toPx()
                        drawLine(colors.border, Offset(0f, y), Offset(size.width, y), strokeWidth = 1.dp.toPx())
                    }
                }
                // The Dashboard's top line comes first for TalkBack, before the actions drawn above it.
                .then(if (root) Modifier.semantics { isTraversalGroup = true; traversalIndex = 1f } else Modifier),
        ) {
            Row(
                Modifier.fillMaxWidth().heightIn(min = 56.dp).padding(horizontal = 4.dp, vertical = if (root) 0.dp else 4.dp),
                // A title that wraps grows the bar downward; the arrow and actions stay level with its first line.
                verticalAlignment = if (root) Alignment.CenterVertically else Alignment.Top,
            ) {
                if (back == null) {
                    Lockup()
                } else {
                    BarAction(AnshinIcon.Back, "Back to ${back.to}", back.go)
                    Text(
                        breakableName(title.orEmpty()),
                        color = colors.text,
                        style = MaterialTheme.typography.titleLarge,
                        modifier = Modifier.weight(1f).padding(start = 4.dp, end = 4.dp, top = 10.dp, bottom = 10.dp)
                            // TalkBack and tests read the title as written, without the break hints.
                            .clearAndSetSemantics { heading(); text = AnnotatedString(title.orEmpty()) },
                    )
                }
                actions()
            }
            pinned?.let {
                Column(Modifier.fillMaxWidth().padding(start = 56.dp, end = 16.dp, bottom = 8.dp)) { it() }
            }
        }
        body(
            Modifier
                .fillMaxWidth()
                .weight(1f)
                .testTag(SCREEN_CONTENT_TAG)
                .then(if (root) Modifier.semantics { isTraversalGroup = true; traversalIndex = 0f } else Modifier),
        )
    }
}

/** The window title already names the app, so TalkBack skips the lockup and starts at the screen's content. */
@Composable
private fun RowScope.Lockup() {
    Spacer(Modifier.width(12.dp))
    AnshinMark()
    Spacer(Modifier.width(8.dp))
    Text("anshin", color = AnshinTheme.colors.text, fontSize = 20.sp, lineHeight = 28.sp, modifier = Modifier.semantics { hideFromAccessibility() })
    Spacer(Modifier.weight(1f))
}

/** A 48 dp icon button in the bar; [label] is what TalkBack reads and what a test finds it by. */
@Composable
internal fun BarAction(icon: AnshinIcon, label: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val colors = AnshinTheme.colors
    IconButton(
        onClick = onClick,
        modifier = modifier.size(48.dp),
        colors = IconButtonDefaults.iconButtonColors(contentColor = colors.text),
    ) {
        Glyph(icon, colors.text, contentDescription = label)
    }
}
