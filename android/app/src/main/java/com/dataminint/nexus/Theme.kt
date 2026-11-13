package com.dataminint.nexus

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

/** Brand palette: indigo → violet → cyan, with semantic status colours. */
object Brand {
    val Indigo = Color(0xFF4F46E5)
    val Violet = Color(0xFF7C3AED)
    val Fuchsia = Color(0xFFC026D3)
    val Cyan = Color(0xFF06B6D4)
    val Sky = Color(0xFF0EA5E9)
    val Teal = Color(0xFF14B8A6)
    val Emerald = Color(0xFF10B981)
    val Green = Color(0xFF22C55E)
    val Amber = Color(0xFFF59E0B)
    val Orange = Color(0xFFF97316)
    val Rose = Color(0xFFEF4444)
    val Crimson = Color(0xFFBE123C)
    val Slate = Color(0xFF64748B)
    val SlateDark = Color(0xFF334155)

    val Gradient = Brush.linearGradient(listOf(Indigo, Violet, Cyan))
    fun grad(a: Color, b: Color) = Brush.linearGradient(listOf(a, b))
}

private val LightColors = lightColorScheme(
    primary = Brand.Indigo, onPrimary = Color.White,
    primaryContainer = Color(0xFFE0E7FF), onPrimaryContainer = Color(0xFF1E1B6E),
    secondary = Color(0xFF0891B2), tertiary = Brand.Violet,
    background = Color(0xFFF4F5FB), onBackground = Color(0xFF14152B),
    surface = Color.White, onSurface = Color(0xFF14152B),
    surfaceVariant = Color(0xFFE9EBF7), onSurfaceVariant = Color(0xFF5B5F7A),
    outline = Color(0xFFC7CAE0), error = Brand.Rose
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF8B93FF), onPrimary = Color(0xFF14123F),
    primaryContainer = Color(0xFF2B2D6B), onPrimaryContainer = Color(0xFFE0E7FF),
    secondary = Color(0xFF22D3EE), tertiary = Color(0xFFA78BFA),
    background = Color(0xFF0E0F1F), onBackground = Color(0xFFE8E9F6),
    surface = Color(0xFF181A30), onSurface = Color(0xFFE8E9F6),
    surfaceVariant = Color(0xFF262945), onSurfaceVariant = Color(0xFFA8ACCB),
    outline = Color(0xFF3A3E63), error = Color(0xFFFF6B6B)
)

private val NexusTypography = Typography().let { t ->
    t.copy(
        headlineMedium = t.headlineMedium.copy(fontWeight = FontWeight.ExtraBold),
        headlineSmall = t.headlineSmall.copy(fontWeight = FontWeight.Bold),
        titleLarge = t.titleLarge.copy(fontWeight = FontWeight.Bold),
        titleMedium = t.titleMedium.copy(fontWeight = FontWeight.SemiBold),
        labelLarge = t.labelLarge.copy(fontWeight = FontWeight.SemiBold)
    )
}

@Composable
fun NexusTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = if (dark) DarkColors else LightColors,
        typography = NexusTypography,
        shapes = Shapes(
            small = RoundedCornerShape(10.dp), medium = RoundedCornerShape(16.dp), large = RoundedCornerShape(24.dp)
        ),
        content = content
    )
}
