package io.github.fsrodrig.anshin

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TextFieldColors
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

internal val ControlShape = RoundedCornerShape(8.dp)

private val ButtonPadding = PaddingValues(horizontal = 20.dp, vertical = 8.dp)

/**
 * The one filled button on a screen. It is filled with the text token, not the accent: the accent means the brand,
 * navigation, selection and Live, and a Save is none of those.
 */
@Composable
internal fun PrimaryButton(label: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true) {
    val colors = AnshinTheme.colors
    Button(
        onClick = onClick,
        modifier = modifier.heightIn(min = 48.dp),
        enabled = enabled,
        shape = ControlShape,
        colors = ButtonDefaults.buttonColors(
            containerColor = colors.text,
            contentColor = colors.background,
            disabledContainerColor = colors.border,
            disabledContentColor = colors.muted,
        ),
        contentPadding = ButtonPadding,
    ) { Text(label, style = MaterialTheme.typography.labelLarge) }
}

@Composable
internal fun SecondaryButton(label: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true) {
    val colors = AnshinTheme.colors
    OutlinedButton(
        onClick = onClick,
        modifier = modifier.heightIn(min = 48.dp),
        enabled = enabled,
        shape = ControlShape,
        border = BorderStroke(1.dp, colors.border),
        colors = ButtonDefaults.outlinedButtonColors(
            contentColor = colors.text,
            disabledContainerColor = colors.border,
            disabledContentColor = colors.muted,
        ),
        contentPadding = ButtonPadding,
    ) { Text(label, style = MaterialTheme.typography.labelLarge) }
}

@Composable
internal fun QuietButton(label: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true) {
    val colors = AnshinTheme.colors
    TextButton(
        onClick = onClick,
        modifier = modifier.heightIn(min = 48.dp),
        enabled = enabled,
        shape = ControlShape,
        colors = ButtonDefaults.textButtonColors(contentColor = colors.text, disabledContentColor = colors.muted),
        contentPadding = ButtonPadding,
    ) { Text(label, style = MaterialTheme.typography.labelLarge) }
}

/** An invalid field turns its border, label and message the text token; its `isError` semantics say the rest. */
@Composable
internal fun anshinTextFieldColors(): TextFieldColors {
    val colors = AnshinTheme.colors
    return OutlinedTextFieldDefaults.colors(
        focusedTextColor = colors.text,
        unfocusedTextColor = colors.text,
        errorTextColor = colors.text,
        cursorColor = colors.accent,
        errorCursorColor = colors.accent,
        focusedBorderColor = colors.accent,
        unfocusedBorderColor = colors.border,
        errorBorderColor = colors.text,
        focusedLabelColor = colors.muted,
        unfocusedLabelColor = colors.muted,
        errorLabelColor = colors.text,
        focusedPlaceholderColor = colors.muted,
        unfocusedPlaceholderColor = colors.muted,
        errorPlaceholderColor = colors.muted,
        focusedSupportingTextColor = colors.muted,
        unfocusedSupportingTextColor = colors.muted,
        errorSupportingTextColor = colors.text,
    )
}
